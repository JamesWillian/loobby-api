package com.jammes.loobby.groups.repo

import com.jammes.loobby.groups.model.GroupEntity
import org.springframework.data.jpa.repository.JpaRepository
import java.util.UUID

interface GroupRepository : JpaRepository<GroupEntity, UUID> {

    fun findByOwnerId(ownerId: UUID): List<GroupEntity>

    fun existsByInviteCode(inviteCode: String): Boolean
}
