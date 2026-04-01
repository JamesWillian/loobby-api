package com.jammes.loobby.groups.service

import com.jammes.loobby.common.files.FileStorageService
import com.jammes.loobby.events.model.EventType
import com.jammes.loobby.events.repo.EventRepository
import com.jammes.loobby.events.repo.EventRsvpRepository
import com.jammes.loobby.events.repo.GameplayEventRepository
import com.jammes.loobby.events.repo.SportEventRepository
import com.jammes.loobby.groups.dto.CreateGroupRequest
import com.jammes.loobby.groups.dto.GroupResponse
import com.jammes.loobby.groups.dto.UpdateGroupRequest
import com.jammes.loobby.groups.model.GroupEntity
import com.jammes.loobby.groups.repo.GroupMemberRepository
import com.jammes.loobby.groups.repo.GroupRepository
import org.springframework.security.access.AccessDeniedException
import org.springframework.stereotype.Service
import org.springframework.transaction.annotation.Transactional
import org.springframework.web.multipart.MultipartFile
import java.util.UUID

@Service
class GroupService(
    private val groupRepository: GroupRepository,
    private val groupMemberRepository: GroupMemberRepository,
    private val fileStorageService: FileStorageService
) {

    fun createGroup(ownerId: UUID, req: CreateGroupRequest): GroupResponse {
        val inviteCode = generateInviteCode()

        val group = groupRepository.save(
            GroupEntity(
                name = req.name,
                inviteCode = inviteCode,
                imageUrl = req.imageUrl,
                ownerId = ownerId
            )
        )

        return group.toResponse()
    }

    fun listGroupsForUser(userId: UUID): List<GroupResponse> {
        val owned = groupRepository.findByOwnerId(userId)

        val memberships = groupMemberRepository.findByUserId(userId)
        val memberGroupIds = memberships.map { it.groupId }.toSet()

        val memberGroups = if (memberGroupIds.isNotEmpty()) {
            groupRepository.findAllById(memberGroupIds)
        } else {
            emptyList()
        }

        // junta os dois sem duplicar
        return (owned + memberGroups)
            .distinctBy { it.id }
            .map { it.toResponse() }
    }


    fun getById(id: UUID): GroupResponse {
        val group = groupRepository.findById(id)
            .orElseThrow { IllegalArgumentException("Group not found") }

        return group.toResponse()
    }

    fun getByCode(code: String): GroupResponse {
        val group = groupRepository.findByInviteCode(code)

        return group?.toResponse() ?: throw IllegalStateException("Invalid Invite Code")
    }

    // ========================= updateGroupName =========================

    /**
     * Altera o nome do grupo. Apenas o dono do grupo pode fazer isso.
     */
    @Transactional
    fun updateGroupName(userId: UUID, groupId: UUID, req: UpdateGroupRequest): GroupResponse {
        val group = groupRepository.findById(groupId)
            .orElseThrow { IllegalArgumentException("Group not found") }

        assertIsGroupOwner(userId, group)

        group.name = req.name
        groupRepository.save(group)

        return group.toResponse()
    }

    // ========================= updateGroupImage =========================

    /**
     * Faz upload de uma nova imagem do grupo. Apenas o dono do grupo pode fazer isso.
     * Segue o mesmo padrão do upload de avatar do usuário (FileStorageService).
     */
    @Transactional
    fun updateGroupImage(userId: UUID, groupId: UUID, file: MultipartFile): GroupResponse {
        val group = groupRepository.findById(groupId)
            .orElseThrow { IllegalArgumentException("Group not found") }

        assertIsGroupOwner(userId, group)

        val imageUrl = fileStorageService.store(
            file = file,
            subfolder = "groups",
            ownerId = groupId       // usa o groupId como "dono" do arquivo
        )

        group.imageUrl = imageUrl
        groupRepository.save(group)

        return group.toResponse()
    }

    // ========================= deleteGroup =========================

    /**
     * Exclui o grupo e todos os dados relacionados.
     * Apenas o dono do grupo pode excluir.
     */
    @Transactional
    fun deleteGroup(userId: UUID, groupId: UUID) {
        val group = groupRepository.findById(groupId)
            .orElseThrow { IllegalArgumentException("Group not found") }

        assertIsGroupOwner(userId, group)

        groupRepository.delete(group)
    }

    // ========================= helpers =========================

    private fun assertIsGroupOwner(userId: UUID, group: GroupEntity) {
        if (group.ownerId != userId) {
            throw AccessDeniedException("Only the group owner can perform this action")
        }
    }

    private fun generateInviteCode(): String {
        // $ + 6 letras maiúsculas e números
        var code: String
        do {
            code = (1..6)
                .map { (('A'..'Z')+(0..9)).random() }
                .joinToString("")
        } while (groupRepository.existsByInviteCode(code))

        return "$-$code"
    }

    private fun GroupEntity.toResponse() =
        GroupResponse(
            id = id,
            name = name,
            inviteCode = inviteCode,
            imageUrl = imageUrl,
            ownerId = ownerId,
            createdAt = createdAt
        )
}
