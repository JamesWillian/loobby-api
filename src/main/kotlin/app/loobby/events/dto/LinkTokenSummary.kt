package app.loobby.events.dto

import app.loobby.events.model.EventLinkTokenEntity
import java.time.Instant
import java.util.UUID

/**
 * Resumo de um link público para listagem no app do host.
 *
 * NÃO inclui o token cru nem o hash — o valor cru só existe no momento da
 * geração e o hash é detalhe interno. O host identifica os tokens por
 * [id], [createdAt] e [status].
 */
data class LinkTokenSummary(
    val id: UUID,
    val eventId: UUID,
    val createdByUserId: UUID,
    val createdAt: Instant?,
    val expiresAt: Instant,
    val revokedAt: Instant?,
    val status: LinkTokenStatus
) {
    companion object {
        fun from(entity: EventLinkTokenEntity, now: Instant = Instant.now()): LinkTokenSummary {
            val status = when {
                entity.revokedAt != null -> LinkTokenStatus.REVOKED
                !now.isBefore(entity.expiresAt) -> LinkTokenStatus.EXPIRED
                else -> LinkTokenStatus.ACTIVE
            }
            return LinkTokenSummary(
                id = entity.id,
                eventId = entity.eventId,
                createdByUserId = entity.createdByUserId,
                createdAt = entity.createdAt,
                expiresAt = entity.expiresAt,
                revokedAt = entity.revokedAt,
                status = status
            )
        }
    }
}

enum class LinkTokenStatus {
    /** Ainda dentro do prazo e não revogado. */
    ACTIVE,

    /** Passou de expires_at sem ter sido revogado. */
    EXPIRED,

    /** Revogado manualmente pelo host. */
    REVOKED
}
