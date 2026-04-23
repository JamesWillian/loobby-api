package app.loobby.notifications.model

import jakarta.persistence.Column
import jakarta.persistence.Entity
import jakarta.persistence.EnumType
import jakarta.persistence.Enumerated
import jakarta.persistence.Id
import jakarta.persistence.Table
import java.time.Instant
import java.time.LocalDate
import java.util.UUID

/**
 * Log de notificações já disparadas — usado como chave de idempotência
 * para jobs diários (EVENT_TODAY_PENDING_RSVP, PAYMENT_DUE_DAILY) e
 * também como mecanismo de debounce para RSVP_CONFIRMED_BY_PEER (caso a data
 * bata com uma execução do mesmo dia).
 *
 * Chave única composta: (user_id, notification_type, reference_id, dispatched_date).
 */
@Entity
@Table(name = "notification_dispatch_log")
open class NotificationDispatchLogEntity(

    @Id
    @Column(name = "id", nullable = false)
    var id: UUID = UUID.randomUUID(),

    @Column(name = "user_id", nullable = false)
    var userId: UUID,

    @Enumerated(EnumType.STRING)
    @Column(name = "notification_type", nullable = false)
    var notificationType: NotificationType,

    /** ID do evento, grupo, ou outro recurso relacionado (depende do tipo). */
    @Column(name = "reference_id", nullable = false)
    var referenceId: UUID,

    /** Data (não timestamp) para garantir idempotência diária. */
    @Column(name = "dispatched_date", nullable = false)
    var dispatchedDate: LocalDate,

    @Column(name = "dispatched_at", insertable = false, updatable = false)
    var dispatchedAt: Instant? = null

) {
    protected constructor() : this(
        userId = UUID.randomUUID(),
        notificationType = NotificationType.EVENT_TODAY_PENDING_RSVP,
        referenceId = UUID.randomUUID(),
        dispatchedDate = LocalDate.now()
    )
}
