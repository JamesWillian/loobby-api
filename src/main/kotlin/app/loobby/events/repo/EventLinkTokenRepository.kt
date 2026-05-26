package app.loobby.events.repo

import app.loobby.events.model.EventLinkTokenEntity
import org.springframework.data.jpa.repository.JpaRepository
import java.util.UUID

interface EventLinkTokenRepository : JpaRepository<EventLinkTokenEntity, UUID> {

    /** Busca o token pelo hash SHA-256 do valor cru recebido na URL. */
    fun findByTokenHash(tokenHash: String): EventLinkTokenEntity?

    /** Lista todos os tokens (ativos e revogados) de um evento. */
    fun findByEventId(eventId: UUID): List<EventLinkTokenEntity>
}
