package app.loobby.events.repo

import app.loobby.events.model.SportEventEntity
import org.springframework.data.jpa.repository.JpaRepository
import java.util.UUID

interface SportEventRepository : JpaRepository<SportEventEntity, UUID> {

    fun findByEventId(eventId: UUID): SportEventEntity?
}
