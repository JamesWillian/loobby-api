package com.jammes.loobby.groups.service

import com.jammes.loobby.groups.dto.CreateGroupRequest
import com.jammes.loobby.groups.dto.GroupResponse
import com.jammes.loobby.groups.model.GroupEntity
import com.jammes.loobby.groups.repo.GroupRepository
import org.springframework.stereotype.Service
import java.util.UUID

@Service
class GroupService(
    private val groupRepository: GroupRepository
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

    fun listGroupsForUser(ownerId: UUID): List<GroupResponse> {
        return groupRepository.findByOwnerId(ownerId)
            .map { it.toResponse() }
    }

    fun getById(id: UUID): GroupResponse {
        val group = groupRepository.findById(id)
            .orElseThrow { IllegalArgumentException("Group not found") }

        return group.toResponse()
    }

    private fun generateInviteCode(): String {
        // L + 6 letras maiúsculas e números
        var code: String
        do {
            code = (1..6)
                .map { (('A'..'Z')+(0..9)).random() }
                .joinToString("")
        } while (groupRepository.existsByInviteCode(code))

        return "L-$code"
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
