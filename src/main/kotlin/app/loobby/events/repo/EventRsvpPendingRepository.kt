package app.loobby.events.repo

import app.loobby.events.model.EventRsvpPendingEntity
import org.springframework.data.jpa.repository.JpaRepository
import org.springframework.data.jpa.repository.Modifying
import org.springframework.data.jpa.repository.Query
import org.springframework.data.repository.query.Param
import java.time.Instant
import java.util.UUID

interface EventRsvpPendingRepository : JpaRepository<EventRsvpPendingEntity, UUID> {

    /** Procura uma confirmação pendente específica (evento + telefone). */
    fun findByEventIdAndPhoneE164(eventId: UUID, phoneE164: String): EventRsvpPendingEntity?

    /** Limpa pendentes expirados — chamado periodicamente por um scheduled job. */
    @Modifying
    @Query("DELETE FROM EventRsvpPendingEntity p WHERE p.expiresAt < :now")
    fun deleteExpired(@Param("now") now: Instant): Int
}
