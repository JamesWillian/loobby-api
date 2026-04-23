package app.loobby.notifications.service

import app.loobby.events.model.EventEntity
import app.loobby.events.model.RsvpStatus
import app.loobby.events.repo.EventRsvpRepository
import app.loobby.groups.repo.GroupMemberRepository
import app.loobby.groups.repo.GroupRepository
import app.loobby.notifications.dto.NotificationPayload
import app.loobby.notifications.model.NotificationType
import app.loobby.users.repo.UsersRepository
import org.slf4j.LoggerFactory
import org.springframework.stereotype.Service
import org.springframework.transaction.event.TransactionPhase
import org.springframework.transaction.event.TransactionalEventListener
import java.util.UUID

/**
 * Orquestra as notificações de todos os 5 cenários. Os hooks event-driven
 * (casos 1 e 5) chamam este serviço a partir dos services de domínio;
 * os time-driven (casos 2, 3 e 4) são disparados pelo NotificationScheduler.
 *
 * Regras principais:
 *  - Nunca notifica o próprio ator
 *  - RSVP peer usa [PeerRsvpDebouncer] para agrupar ("João e mais N confirmaram")
 *  - Novo evento em grupo notifica TODOS os membros, inclusive sem interação prévia
 */
@Service
class NotificationService(
    private val fcmSender: FcmSender,
    private val usersRepository: UsersRepository,
    private val eventRsvpRepository: EventRsvpRepository,
    private val groupRepository: GroupRepository,
    private val groupMemberRepository: GroupMemberRepository,
    private val peerRsvpDebouncer: PeerRsvpDebouncer
) {
    private val log = LoggerFactory.getLogger(javaClass)

    // ─────────────────────────────────────────────────────────────────
    // Caso 1: alguém confirmou RSVP=YES em evento que eu também confirmei
    //
    // Estratégia anti-ruído: agrupamento via PeerRsvpDebouncer, que mantém
    // em memória uma janela de debounce por (event + destinatário). Quando
    // a janela fecha, dispara 1 push com mensagem agrupada.
    // ─────────────────────────────────────────────────────────────────
    fun onRsvpConfirmed(event: EventEntity, actorUserId: UUID) {
        try {
            // Busca todos os confirmados (YES) exceto o próprio ator
            val peers = eventRsvpRepository
                .findByEventIdInAndStatus(listOf(event.id), RsvpStatus.YES)
                .map { it.userId }
                .filter { it != actorUserId }
                .toSet()

            if (peers.isEmpty()) return

            val actor = usersRepository.findById(actorUserId).orElse(null) ?: return
            val actorName = actor.displayname ?: actor.username

            peers.forEach { recipientId ->
                peerRsvpDebouncer.schedule(
                    eventId = event.id,
                    recipientId = recipientId,
                    actorId = actorUserId,
                    actorName = actorName,
                    eventName = event.name
                ) { flushedActorNames, extraCount ->
                    val title = event.name
                    val body = buildPeerRsvpMessage(flushedActorNames, extraCount)
                    fcmSender.sendToUser(
                        recipientId,
                        NotificationPayload(
                            title = title,
                            body = body,
                            type = NotificationType.RSVP_CONFIRMED_BY_PEER,
                            data = mapOf(
                                "eventId" to event.id.toString(),
                                "actorId" to actorUserId.toString()
                            )
                        )
                    )
                }
            }
        } catch (t: Throwable) {
            log.error("Failed to dispatch RSVP_CONFIRMED_BY_PEER for event={}", event.id, t)
        }
    }

    /**
     * Monta a mensagem agrupada.
     *  - 1 ator: "João confirmou presença"
     *  - 2 atores: "João e Maria confirmaram presença"
     *  - 3+: "João, Maria e mais N confirmaram presença"
     */
    private fun buildPeerRsvpMessage(names: List<String>, extraCount: Int): String {
        val totalExtras = extraCount
        return when {
            names.size == 1 && totalExtras == 0 -> "${names[0]} confirmou presença"
            names.size == 2 && totalExtras == 0 -> "${names[0]} e ${names[1]} confirmaram presença"
            names.size >= 2 && totalExtras > 0 ->
                "${names[0]}, ${names[1]} e mais $totalExtras confirmaram presença"
            names.size == 1 && totalExtras > 0 ->
                "${names[0]} e mais $totalExtras confirmaram presença"
            else -> "Novas confirmações no evento"
        }
    }

    // ─────────────────────────────────────────────────────────────────
    // Caso 5: novo evento criado em grupo → notifica TODOS os membros
    // ─────────────────────────────────────────────────────────────────
    fun onGroupEventCreated(event: EventEntity, creatorId: UUID) {
        val groupId = event.groupId ?: return

        try {
            val group = groupRepository.findById(groupId).orElse(null) ?: return

            val members = groupMemberRepository.findByGroupId(groupId)
                .map { it.userId }
                .filter { it != creatorId }
                .toSet()

            if (members.isEmpty()) return

            fcmSender.sendToUsers(
                members,
                NotificationPayload(
                    title = group.name,
                    body = "Novo evento: ${event.name}",
                    type = NotificationType.NEW_GROUP_EVENT,
                    data = mapOf(
                        "eventId" to event.id.toString(),
                        "groupId" to groupId.toString()
                    )
                )
            )
        } catch (t: Throwable) {
            log.error("Failed to dispatch NEW_GROUP_EVENT for event={}", event.id, t)
        }
    }

    // ─────────────────────────────────────────────────────────────────
    // Métodos usados pelo NotificationScheduler (Turno 3).
    // Expostos aqui para centralizar a formatação das mensagens.
    // ─────────────────────────────────────────────────────────────────

    fun sendEventTodayPendingRsvp(userId: UUID, event: EventEntity) {
        fcmSender.sendToUser(
            userId,
            NotificationPayload(
                title = "Evento hoje",
                body = "${event.name} é hoje — você ainda não confirmou presença",
                type = NotificationType.EVENT_TODAY_PENDING_RSVP,
                data = mapOf("eventId" to event.id.toString())
            )
        )
    }

    fun sendEventReminderHoursBefore(userId: UUID, event: EventEntity, hoursBefore: Int) {
        fcmSender.sendToUser(
            userId,
            NotificationPayload(
                title = event.name,
                body = "Começa em $hoursBefore horas — se prepare!",
                type = NotificationType.EVENT_REMINDER_HOURS_BEFORE,
                data = mapOf("eventId" to event.id.toString())
            )
        )
    }

    fun sendPaymentDueDaily(userId: UUID, event: EventEntity) {
        fcmSender.sendToUser(
            userId,
            NotificationPayload(
                title = "Pagamento pendente",
                body = "Você ainda não pagou ${event.name}",
                type = NotificationType.PAYMENT_DUE_DAILY,
                data = mapOf("eventId" to event.id.toString())
            )
        )
    }
}
