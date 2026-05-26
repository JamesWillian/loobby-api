package app.loobby.events.scheduler

import app.loobby.events.repo.EventRsvpPendingRepository
import org.slf4j.LoggerFactory
import org.springframework.scheduling.annotation.Scheduled
import org.springframework.stereotype.Component
import org.springframework.transaction.annotation.Transactional
import java.time.Instant

/**
 * Limpa periodicamente as confirmações pendentes que passaram de [expiresAt].
 *
 * Cada chamada a POST /public/c/{token}/rsvps/start cria/atualiza um registro em
 * event_rsvp_pending. Se o convidado não concluir o OTP via Firebase Phone Auth
 * dentro do TTL (loobby.public.pending-rsvp-ttl-hours), o registro vira lixo —
 * este job é quem o varre.
 *
 * O índice ix_event_rsvp_pending_expires garante varredura O(log n) tocando
 * só as linhas vencidas.
 */
@Component
class PendingRsvpCleanupScheduler(
    private val pendingRepository: EventRsvpPendingRepository
) {
    private val log = LoggerFactory.getLogger(javaClass)

    @Scheduled(cron = "\${loobby.public.pending-cleanup-cron:0 0 * * * *}")
    @Transactional
    fun cleanupExpired() {
        val now = Instant.now()
        val deleted = pendingRepository.deleteExpired(now)
        if (deleted > 0) {
            log.info("Cleanup: deleted {} expired event_rsvp_pending rows", deleted)
        }
    }
}
