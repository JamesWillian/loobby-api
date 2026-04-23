package app.loobby.notifications.scheduler

import app.loobby.events.model.EventEntity
import app.loobby.events.model.RsvpStatus
import app.loobby.events.repo.EventRepository
import app.loobby.events.repo.EventRsvpRepository
import app.loobby.events.repo.SportEventRepository
import app.loobby.groups.repo.GroupMemberRepository
import app.loobby.notifications.model.NotificationDispatchLogEntity
import app.loobby.notifications.model.NotificationType
import app.loobby.notifications.repo.NotificationDispatchLogRepository
import app.loobby.notifications.service.NotificationService
import org.slf4j.LoggerFactory
import org.springframework.beans.factory.annotation.Value
import org.springframework.dao.DataIntegrityViolationException
import org.springframework.scheduling.annotation.Scheduled
import org.springframework.stereotype.Component
import java.math.BigDecimal
import java.time.Instant
import java.time.LocalDate
import java.time.LocalTime
import java.time.ZoneId
import java.time.ZonedDateTime
import java.util.UUID

/**
 * Jobs agendados das notificações time-driven (casos 2, 3 e 4).
 *
 * TZ de referência: [loobby.notifications.timezone] (MVP fixo em America/Sao_Paulo).
 * Os crons são configurados em [application.yaml] para permitir ajuste sem deploy.
 *
 * Estratégia anti-duplicata:
 *  - Para EVENT_TODAY_PENDING_RSVP e PAYMENT_DUE_DAILY → [notification_dispatch_log]
 *    com chave (user, type, event, date), garantindo 1 envio/dia mesmo se o job
 *    rodar múltiplas vezes ou se forem múltiplas instâncias.
 *  - Para EVENT_REMINDER_HOURS_BEFORE → idem, usando a data do dia como chave.
 *    Como o lembrete é único por evento, um log com data do evento basta.
 */
@Component
class NotificationScheduler(
    private val eventRepository: EventRepository,
    private val eventRsvpRepository: EventRsvpRepository,
    private val sportEventRepository: SportEventRepository,
    private val groupMemberRepository: GroupMemberRepository,
    private val dispatchLogRepository: NotificationDispatchLogRepository,
    private val notificationService: NotificationService,

    @Value("\${loobby.notifications.timezone}")
    private val timezoneId: String,

    @Value("\${loobby.notifications.reminder-hours-before}")
    private val reminderHoursBefore: Long
) {
    private val log = LoggerFactory.getLogger(javaClass)

    private val zoneId: ZoneId get() = ZoneId.of(timezoneId)

    // ─────────────────────────────────────────────────────────────────
    // Job diário às 08:00 (America/Sao_Paulo)
    // Cobre: EVENT_TODAY_PENDING_RSVP e PAYMENT_DUE_DAILY
    // ─────────────────────────────────────────────────────────────────
    @Scheduled(cron = "\${loobby.notifications.daily-job-cron}", zone = "\${loobby.notifications.timezone}")
    fun dailyMorningJob() {
        val today = LocalDate.now(zoneId)
        log.info("Running dailyMorningJob for date={} tz={}", today, timezoneId)

        try {
            dispatchEventTodayPendingRsvp(today)
        } catch (t: Throwable) {
            log.error("EVENT_TODAY_PENDING_RSVP dispatch failed", t)
        }

        try {
            dispatchPaymentDueDaily(today)
        } catch (t: Throwable) {
            log.error("PAYMENT_DUE_DAILY dispatch failed", t)
        }
    }

    /**
     * Caso 2: para cada evento de hoje (grupo), pega todos os membros sem RSVP
     * ou com RSVP != YES e dispara notificação (1x por dia por par user+event).
     */
    private fun dispatchEventTodayPendingRsvp(today: LocalDate) {
        val (dayStart, dayEnd) = dayBoundsUtc(today)
        val todayEvents = eventRepository.findByScheduledDatetimeBetween(dayStart, dayEnd)
        if (todayEvents.isEmpty()) return

        todayEvents.forEach { event ->
            val groupId = event.groupId ?: return@forEach // ignora eventos instantâneos

            val memberIds = groupMemberRepository.findByGroupId(groupId)
                .map { it.userId }
                .toSet()
            if (memberIds.isEmpty()) return@forEach

            val allRsvps = eventRsvpRepository.findByEventIdOrderByCreatedAtAsc(event.id)
            val confirmedOrDeclinedUserIds = allRsvps
                .filter { it.status == RsvpStatus.YES || it.status == RsvpStatus.NO }
                .map { it.userId }
                .toSet()

            val pendingUserIds = memberIds - confirmedOrDeclinedUserIds

            pendingUserIds.forEach { userId ->
                tryDispatch(
                    userId = userId,
                    type = NotificationType.EVENT_TODAY_PENDING_RSVP,
                    referenceId = event.id,
                    date = today
                ) {
                    notificationService.sendEventTodayPendingRsvp(userId, event)
                }
            }
        }
    }

    /**
     * Caso 4: para cada RSVP=YES em evento futuro (inclusive hoje) com preço > 0
     * e is_paid=false, dispara notificação de pagamento (1x por dia).
     */
    private fun dispatchPaymentDueDaily(today: LocalDate) {
        val nowUtc = Instant.now()
        val windowEnd = nowUtc.plusSeconds(60L * 60L * 24L * 30L) // próximos 30 dias

        val upcomingEvents = eventRepository.findByScheduledDatetimeBetween(nowUtc, windowEnd)
        if (upcomingEvents.isEmpty()) return

        val eventIds = upcomingEvents.map { it.id }
        val sports = sportEventRepository.findAllByEventIdIn(eventIds)
            .filter { it.pricePerPlayer > BigDecimal.ZERO }
            .associateBy { it.eventId }
        if (sports.isEmpty()) return

        val paidEventIds = sports.keys
        val pendingRsvps = eventRsvpRepository
            .findByEventIdInAndStatusAndIsPaidFalse(paidEventIds, RsvpStatus.YES)

        val eventsById = upcomingEvents.associateBy { it.id }

        pendingRsvps.forEach { rsvp ->
            val event = eventsById[rsvp.eventId] ?: return@forEach
            tryDispatch(
                userId = rsvp.userId,
                type = NotificationType.PAYMENT_DUE_DAILY,
                referenceId = event.id,
                date = today
            ) {
                notificationService.sendPaymentDueDaily(rsvp.userId, event)
            }
        }
    }

    // ─────────────────────────────────────────────────────────────────
    // Job de lembrete (a cada 15 min)
    // Dispara EVENT_REMINDER_HOURS_BEFORE para eventos confirmados
    // cuja hora de início está a ~reminderHoursBefore horas de agora.
    // ─────────────────────────────────────────────────────────────────
    @Scheduled(cron = "\${loobby.notifications.reminder-job-cron}", zone = "\${loobby.notifications.timezone}")
    fun reminderJob() {
        val now = Instant.now()
        val windowStart = now.plusSeconds(reminderHoursBefore * 3600)
        val windowEnd = windowStart.plusSeconds(15 * 60) // 15 min de janela (mesmo período do cron)

        val upcoming = eventRepository.findByScheduledDatetimeBetween(windowStart, windowEnd)
        if (upcoming.isEmpty()) return

        log.info(
            "reminderJob found {} events in window {}..{}", upcoming.size, windowStart, windowEnd
        )

        upcoming.forEach { event ->
            val confirmedRsvps = eventRsvpRepository
                .findByEventIdInAndStatus(listOf(event.id), RsvpStatus.YES)
            if (confirmedRsvps.isEmpty()) return@forEach

            val eventDay = event.scheduledDatetime.atZone(zoneId).toLocalDate()

            confirmedRsvps.forEach { rsvp ->
                tryDispatch(
                    userId = rsvp.userId,
                    type = NotificationType.EVENT_REMINDER_HOURS_BEFORE,
                    referenceId = event.id,
                    date = eventDay
                ) {
                    notificationService.sendEventReminderHoursBefore(
                        rsvp.userId, event, reminderHoursBefore.toInt()
                    )
                }
            }
        }
    }

    // ─────────────────────────────────────────────────────────────────
    // Helpers
    // ─────────────────────────────────────────────────────────────────

    /**
     * Dispara o push somente se não houver registro no [notification_dispatch_log]
     * para a tripla (user, type, event, date).
     *
     * Usa a constraint UNIQUE como mecanismo de trava distribuída: se duas instâncias
     * tentam disparar ao mesmo tempo, uma vence e a outra cai em
     * [DataIntegrityViolationException] e só loga. Cada save() do JpaRepository é
     * atômico/transacional por padrão, então não precisamos de @Transactional aqui
     * (que, aliás, não seria aplicado via proxy por ser chamada intra-classe).
     */
    private fun tryDispatch(
        userId: UUID,
        type: NotificationType,
        referenceId: UUID,
        date: LocalDate,
        block: () -> Unit
    ) {
        val already = dispatchLogRepository
            .existsByUserIdAndNotificationTypeAndReferenceIdAndDispatchedDate(
                userId, type, referenceId, date
            )
        if (already) return

        try {
            dispatchLogRepository.save(
                NotificationDispatchLogEntity(
                    userId = userId,
                    notificationType = type,
                    referenceId = referenceId,
                    dispatchedDate = date
                )
            )
        } catch (e: DataIntegrityViolationException) {
            // Outra instância já gravou o log — ignore e não dispara.
            log.debug(
                "Skipped duplicate dispatch user={} type={} ref={} date={}",
                userId, type, referenceId, date
            )
            return
        }

        try {
            block()
        } catch (t: Throwable) {
            log.error(
                "Notification send failed after log user={} type={} ref={}",
                userId, type, referenceId, t
            )
        }
    }

    /** Limites UTC do dia [date] no timezone do app. */
    private fun dayBoundsUtc(date: LocalDate): Pair<Instant, Instant> {
        val startLocal = ZonedDateTime.of(date, LocalTime.MIN, zoneId)
        val endLocal = ZonedDateTime.of(date, LocalTime.MAX, zoneId)
        return startLocal.toInstant() to endLocal.toInstant()
    }
}
