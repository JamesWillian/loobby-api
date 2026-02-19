package com.jammes.loobby.events.repo

import com.jammes.loobby.events.model.SportEventEntity
import org.springframework.data.jpa.repository.JpaRepository
import java.util.UUID

interface SportEventRepository : JpaRepository<SportEventEntity, UUID> {

    fun findByEventId(eventId: UUID): SportEventEntity?
}
