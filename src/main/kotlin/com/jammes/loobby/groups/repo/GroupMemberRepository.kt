package com.jammes.loobby.groups.repo

import com.jammes.loobby.groups.model.GroupMemberEntity
import org.springframework.data.jpa.repository.JpaRepository
import java.util.UUID

interface GroupMemberRepository : JpaRepository<GroupMemberEntity, UUID> {

    fun existsByGroupIdAndUserId(groupId: UUID, userId: UUID): Boolean

    fun findByGroupId(groupId: UUID): List<GroupMemberEntity>

    fun findByUserId(userId: UUID): List<GroupMemberEntity>

    fun deleteByGroupIdAndUserId(groupId: UUID, userId: UUID)
}
