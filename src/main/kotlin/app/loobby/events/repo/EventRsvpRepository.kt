package app.loobby.events.repo

import app.loobby.events.model.EventRsvpEntity
import app.loobby.events.model.EventRsvpId
import app.loobby.events.model.RsvpStatus
import org.springframework.data.jpa.repository.JpaRepository
import org.springframework.data.jpa.repository.Modifying
import org.springframework.data.jpa.repository.Query
import org.springframework.data.repository.query.Param
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

    /** Usado pelo job de pagamento pendente: busca RSVPs confirmados e não pagos em eventos futuros. */
    fun findByEventIdInAndStatusAndIsPaidFalse(
        eventIds: Collection<UUID>,
        status: RsvpStatus
    ): List<EventRsvpEntity>

    /** Quantos RSVPs em um determinado status (ex.: confirmados=YES) o evento tem. */
    fun countByEventIdAndStatus(eventId: UUID, status: RsvpStatus): Long

    /**
     * Reatribui RSVPs do usuário [fromUserId] para [toUserId], pulando os
     * eventos em que [toUserId] já tem RSVP (evita violar a PK composta).
     *
     * Usado no fluxo de promoção lite → full pelo [app.loobby.users.service.UserMergeService].
     */
    @Modifying
    @Query(
        value = """
            UPDATE event_rsvps
            SET user_id = :toUserId
            WHERE user_id = :fromUserId
              AND event_id NOT IN (
                SELECT event_id FROM event_rsvps WHERE user_id = :toUserId
              )
        """,
        nativeQuery = true
    )
    fun reassignUserIfNoConflict(
        @Param("fromUserId") fromUserId: UUID,
        @Param("toUserId") toUserId: UUID
    ): Int

    /** Remove todas as RSVPs de um usuário — usado para limpar sobras após merge. */
    @Modifying
    @Query("DELETE FROM EventRsvpEntity r WHERE r.userId = :userId")
    fun deleteByUserId(@Param("userId") userId: UUID): Int
}
