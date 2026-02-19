package com.jammes.loobby.groups.service

import com.jammes.loobby.groups.dto.GroupMemberResponse
import com.jammes.loobby.groups.model.GroupMemberEntity
import com.jammes.loobby.groups.repo.GroupMemberRepository
import com.jammes.loobby.groups.repo.GroupRepository
import com.jammes.loobby.users.repo.UsersRepository
import org.springframework.stereotype.Service
import org.springframework.transaction.annotation.Transactional
import java.util.UUID

@Service
class GroupMemberService(
    private val groupRepository: GroupRepository,
    private val groupMemberRepository: GroupMemberRepository,
    private val usersRepository: UsersRepository
) {

    fun joinGroup(userId: UUID, groupId: UUID) {
        val group = groupRepository.findById(groupId)
            .orElseThrow { IllegalArgumentException("Group not found") }

        // já é membro? não faz nada
        if (groupMemberRepository.existsByGroupIdAndUserId(groupId, userId)) {
            return
        }

        groupMemberRepository.save(
            GroupMemberEntity(
                groupId = group.id,
                userId = userId
            )
        )
    }

    @Transactional
    fun leaveGroup(userId: UUID, groupId: UUID) {
        val group = groupRepository.findById(groupId)
            .orElseThrow { IllegalArgumentException("Group not found") }

        // regra simples: dono não pode sair do próprio grupo
        if (group.ownerId == userId) {
            throw IllegalStateException("Group owner cannot leave the group")
        }

        groupMemberRepository.deleteByGroupIdAndUserId(groupId, userId)
    }

    fun listMembers(groupId: UUID): List<GroupMemberResponse> {
        val group = groupRepository.findById(groupId)
            .orElseThrow { IllegalArgumentException("Group not found") }

        val members = groupMemberRepository.findByGroupId(groupId)
        if (members.isEmpty()) return emptyList()

        val userIds = members.map { it.userId }
        val users = usersRepository.findAllById(userIds)
        val usersById = users.associateBy { it.id }

        return members.mapNotNull { member ->
            val user = usersById[member.userId] ?: return@mapNotNull null

            GroupMemberResponse(
                userId = user.id,
                username = user.username,
                displayname = user.displayname,
                avatarUrl = user.avatarUrl,
                isOwner = (user.id == group.ownerId)
            )
        }
    }
}
