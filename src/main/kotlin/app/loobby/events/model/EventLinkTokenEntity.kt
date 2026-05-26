package app.loobby.events.model

import jakarta.persistence.*
import java.time.Instant
import java.util.UUID

/**
 * Token público compartilhável de um evento — base do link
 * https://loobby.app/c/<token>.
 *
 * O token bruto NUNCA é armazenado: guardamos apenas seu SHA-256 em [tokenHash].
 * Para validar uma requisição, o servidor hasheia o token recebido e procura aqui.
 *
 * Um token deixa de ser válido quando passa de [expiresAt] OU quando [revokedAt]
 * é preenchido (revogação manual pelo host).
 */
@Entity
@Table(
    name = "event_link_tokens",
    indexes = [
        Index(name = "ix_event_link_tokens_event", columnList = "event_id")
    ]
)
open class EventLinkTokenEntity(

    @Id
    @Column(name = "id", nullable = false)
    var id: UUID = UUID.randomUUID(),

    @Column(name = "event_id", nullable = false)
    var eventId: UUID,

    @Column(name = "token_hash", nullable = false, unique = true, length = 100)
    var tokenHash: String,

    @Column(name = "created_by_user_id", nullable = false)
    var createdByUserId: UUID,

    @Column(name = "expires_at", nullable = false)
    var expiresAt: Instant,

    @Column(name = "revoked_at", nullable = true)
    var revokedAt: Instant? = null,

    @Column(name = "created_at", insertable = false, updatable = false)
    var createdAt: Instant? = null

) {
    protected constructor() : this(
        id = UUID.randomUUID(),
        eventId = UUID.randomUUID(),
        tokenHash = "",
        createdByUserId = UUID.randomUUID(),
        expiresAt = Instant.now(),
        revokedAt = null,
        createdAt = null
    )

    /** True se o token ainda pode ser usado neste momento. */
    fun isUsable(now: Instant = Instant.now()): Boolean =
        revokedAt == null && now.isBefore(expiresAt)
}
