package app.loobby.groups.repo

import app.loobby.groups.model.GroupEntity
import org.springframework.data.jpa.repository.JpaRepository
import java.util.UUID

interface GroupRepository : JpaRepository<GroupEntity, UUID> {

    fun findByOwnerId(ownerId: UUID): List<GroupEntity>

    fun findByInviteCode(inviteCode: String): GroupEntity?

    fun existsByInviteCode(inviteCode: String): Boolean
}
