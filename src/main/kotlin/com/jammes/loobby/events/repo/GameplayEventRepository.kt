package com.jammes.loobby.events.repo

import com.jammes.loobby.events.model.GameplayEventEntity
import org.springframework.data.jpa.repository.JpaRepository
import java.util.UUID

interface GameplayEventRepository : JpaRepository<GameplayEventEntity, UUID> {

    fun findByEventId(eventId: UUID): GameplayEventEntity?
}
