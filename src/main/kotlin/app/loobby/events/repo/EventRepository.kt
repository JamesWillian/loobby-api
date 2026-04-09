package app.loobby.events.repo

import app.loobby.events.model.EventEntity
import org.springframework.data.jpa.repository.JpaRepository
import java.time.Instant
import java.util.UUID

interface EventRepository : JpaRepository<EventEntity, UUID> {

    fun findByGroupIdOrderByScheduledDatetimeAsc(groupId: UUID): List<EventEntity>

    fun findByOwnerIdOrderByScheduledDatetimeAsc(ownerId: UUID): List<EventEntity>

    fun findByGroupIdAndScheduledDatetimeAfterOrderByScheduledDatetimeAsc(
        groupId: UUID,
        after: Instant
    ): List<EventEntity>

    fun findByIsInstantAndOwnerIdOrderByScheduledDatetimeDesc(
        isInstant: Boolean,
        ownerId: UUID
    ): List<EventEntity>

    fun existsByInviteCode(inviteCode: String): Boolean
}
