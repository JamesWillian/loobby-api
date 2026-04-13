package app.loobby.events.repo

import app.loobby.events.model.EventRsvpEntity
import app.loobby.events.model.EventRsvpId
import app.loobby.events.model.RsvpStatus
import org.springframework.data.jpa.repository.JpaRepository
import org.springframework.data.jpa.repository.Modifying
import org.springframework.data.jpa.repository.Query
import java.util.UUID

interface EventRsvpRepository : JpaRepository<EventRsvpEntity, EventRsvpId> {

    // Retorna o participante confirmado mais antigo de um evento, excluindo o owner atual
    // Usado para encontrar o próximo owner de eventos instantâneos ao excluir conta
    fun findFirstByEventIdAndUserIdNotAndStatusOrderByCreatedAtAsc(
        eventId: UUID,
        userId: UUID,
        status: RsvpStatus
    ): EventRsvpEntity?

    // Zera o campo obs de todos os RSVPs do usuário que está excluindo a conta
    @Modifying
    @Query("UPDATE EventRsvpEntity r SET r.obs = null WHERE r.userId = :userId")
    fun clearObsByUserId(userId: UUID)

    fun findByEventIdOrderByCreatedAtAsc(eventId: UUID): List<EventRsvpEntity>

    fun findByEventIdAndUserId(eventId: UUID, userId: UUID): EventRsvpEntity?

    fun findByEventIdInAndUserId(eventIds: Collection<UUID>, userId: UUID): List<EventRsvpEntity>

    fun findByEventIdInAndStatus(eventIds: Collection<UUID>, status: RsvpStatus): List<EventRsvpEntity>
}
