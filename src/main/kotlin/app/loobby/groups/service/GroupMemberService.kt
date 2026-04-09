package app.loobby.groups.service

import app.loobby.groups.dto.GroupMemberResponse
import app.loobby.groups.model.GroupMemberEntity
import app.loobby.groups.repo.GroupMemberRepository
import app.loobby.groups.repo.GroupRepository
import app.loobby.users.repo.UsersRepository
import org.springframework.security.access.AccessDeniedException
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

    // ========================= removeMember =========================

    /**
     * Remove um membro do grupo. Apenas o dono do grupo pode fazer isso.
     * O dono não pode remover a si próprio.
     */
    @Transactional
    fun removeMember(requesterId: UUID, groupId: UUID, memberId: UUID) {
        val group = groupRepository.findById(groupId)
            .orElseThrow { IllegalArgumentException("Group not found") }

        // Apenas o dono do grupo pode remover membros
        if (group.ownerId != requesterId) {
            throw AccessDeniedException("Only the group owner can remove members")
        }

        // Dono não pode se remover por esta rota
        if (memberId == group.ownerId) {
            throw IllegalStateException("Cannot remove the group owner.")
        }

        // Verifica se o usuário é membro
        if (!groupMemberRepository.existsByGroupIdAndUserId(groupId, memberId)) {
            throw IllegalArgumentException("User is not a member of this group")
        }

        groupMemberRepository.deleteByGroupIdAndUserId(groupId, memberId)
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
