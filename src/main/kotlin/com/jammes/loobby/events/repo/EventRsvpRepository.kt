package com.jammes.loobby.events.repo

import com.jammes.loobby.events.model.EventRsvpEntity
import com.jammes.loobby.events.model.EventRsvpId
import com.jammes.loobby.events.model.RsvpStatus
import org.springframework.data.jpa.repository.JpaRepository
import java.util.UUID

interface EventRsvpRepository : JpaRepository<EventRsvpEntity, EventRsvpId> {

    fun findByEventIdOrderByCreatedAtAsc(eventId: UUID): List<EventRsvpEntity>

    fun findByEventIdAndUserId(eventId: UUID, userId: UUID): EventRsvpEntity?

    fun findByEventIdInAndUserId(eventIds: Collection<UUID>, userId: UUID): List<EventRsvpEntity>

    fun findByEventIdInAndStatus(eventIds: Collection<UUID>, status: RsvpStatus): List<EventRsvpEntity>

    fun existsByEventIdAndUserId(eventId: UUID, userId: UUID): Boolean

    fun deleteByEventIdAndUserId(eventId: UUID, userId: UUID)
}
