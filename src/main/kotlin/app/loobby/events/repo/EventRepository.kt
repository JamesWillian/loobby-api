package app.loobby.events.repo

import app.loobby.events.model.EventEntity
import org.springframework.data.jpa.repository.JpaRepository
import java.time.Instant
import java.util.UUID

interface EventRepository : JpaRepository<EventEntity, UUID> {

    fun findByOwnerIdAndIsInstantTrue(ownerId: UUID): List<EventEntity>

    fun findByGroupIdOrderByScheduledDatetimeAsc(groupId: UUID): List<EventEntity>

    fun existsByInviteCode(inviteCode: String): Boolean

    fun findByInviteCode(inviteCode: String): EventEntity?
}
