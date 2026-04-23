package app.loobby.notifications.repo

import app.loobby.notifications.model.NotificationDispatchLogEntity
import app.loobby.notifications.model.NotificationType
import org.springframework.data.jpa.repository.JpaRepository
import java.time.LocalDate
import java.util.UUID

interface NotificationDispatchLogRepository : JpaRepository<NotificationDispatchLogEntity, UUID> {

    /**
     * Verifica se já foi enviada uma notificação deste tipo para esse user + referência
     * nesta data. Usado para idempotência do job diário e debounce do peer-RSVP.
     */
    fun existsByUserIdAndNotificationTypeAndReferenceIdAndDispatchedDate(
        userId: UUID,
        notificationType: NotificationType,
        referenceId: UUID,
        dispatchedDate: LocalDate
    ): Boolean
}
