package app.loobby.events.model

import jakarta.persistence.*
import org.hibernate.annotations.UpdateTimestamp
import java.time.Instant
import java.util.UUID

/**
 * Confirmação de presença aguardando verificação por OTP (Firebase Phone Auth).
 *
 * Diferente de [EventRsvpEntity], NÃO referencia users — guardamos apenas
 * o telefone e o nome digitados no formulário público. O usuário só é criado
 * (ou vinculado ao já existente pelo phone_e164) no momento em que a
 * confirmação for promovida para event_rsvps, evitando poluir a tabela users
 * com gente que nunca terminou de confirmar.
 *
 * Ciclo de vida:
 *  1. POST /public/c/{token}/rsvps/start  → cria/atualiza esta linha
 *  2. Frontend dispara Firebase Phone Auth → SMS → usuário digita código
 *  3. POST /public/c/{token}/rsvps/confirm → promove para event_rsvps
 *     e apaga esta linha
 *  4. Se o passo 3 não acontecer dentro de [expiresAt], o
 *     PendingRsvpCleanupScheduler descarta a linha.
 */
@Entity
@Table(
    name = "event_rsvp_pending",
    indexes = [
        Index(name = "ix_event_rsvp_pending_phone", columnList = "phone_e164"),
        Index(name = "ix_event_rsvp_pending_expires", columnList = "expires_at")
    ],
    uniqueConstraints = [
        UniqueConstraint(
            name = "event_rsvp_pending_event_phone_unique",
            columnNames = ["event_id", "phone_e164"]
        )
    ]
)
open class EventRsvpPendingEntity(

    @Id
    @Column(name = "id", nullable = false)
    var id: UUID = UUID.randomUUID(),

    @Column(name = "event_id", nullable = false)
    var eventId: UUID,

    @Column(name = "phone_e164", nullable = false, length = 20)
    var phoneE164: String,

    @Column(name = "display_name", nullable = false, length = 100)
    var displayName: String,

    /** Timestamp da última chamada de /rsvps/start para esse telefone+evento. */
    @Column(name = "otp_sent_at", nullable = true)
    var otpSentAt: Instant? = null,

    /** Quantas vezes /rsvps/start foi chamado para esse pending (para anti-abuso). */
    @Column(name = "otp_attempts", nullable = false)
    var otpAttempts: Int = 0,

    @Column(name = "expires_at", nullable = false)
    var expiresAt: Instant,

    @Column(name = "created_at", insertable = false, updatable = false)
    var createdAt: Instant? = null,

    @UpdateTimestamp
    @Column(name = "updated_at", nullable = false)
    var updatedAt: Instant = Instant.now()

) {
    protected constructor() : this(
        id = UUID.randomUUID(),
        eventId = UUID.randomUUID(),
        phoneE164 = "",
        displayName = "",
        otpSentAt = null,
        otpAttempts = 0,
        expiresAt = Instant.now(),
        createdAt = null,
        updatedAt = Instant.now()
    )
}
