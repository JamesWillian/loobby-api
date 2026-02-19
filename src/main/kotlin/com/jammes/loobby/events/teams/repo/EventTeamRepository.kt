package com.jammes.loobby.events.teams.repo

import com.jammes.loobby.events.teams.model.EventTeamEntity
import org.springframework.data.jpa.repository.JpaRepository
import java.util.UUID

interface EventTeamRepository : JpaRepository<EventTeamEntity, UUID> {

    fun findByEventIdOrderByOrderIndexAsc(eventId: UUID): List<EventTeamEntity>

    fun deleteByEventId(eventId: UUID)
}
