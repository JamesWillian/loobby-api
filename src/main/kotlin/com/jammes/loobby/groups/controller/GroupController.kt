package com.jammes.loobby.groups.controller

import com.jammes.loobby.groups.dto.CreateGroupRequest
import com.jammes.loobby.groups.dto.GroupMemberResponse
import com.jammes.loobby.groups.dto.GroupResponse
import com.jammes.loobby.groups.service.GroupMemberService
import com.jammes.loobby.groups.service.GroupService
import org.springframework.security.core.annotation.AuthenticationPrincipal
import org.springframework.security.oauth2.jwt.Jwt
import org.springframework.web.bind.annotation.*
import java.util.UUID
import jakarta.validation.Valid

@RestController
@RequestMapping("/groups")
class GroupController(
    private val groupService: GroupService,
    private val groupMemberService: GroupMemberService
) {

    @PostMapping
    fun createGroup(
        @AuthenticationPrincipal jwt: Jwt,
        @Valid @RequestBody req: CreateGroupRequest
    ): GroupResponse {
        val userId = UUID.fromString(jwt.subject)
        return groupService.createGroup(userId, req)
    }

    @GetMapping
    fun listMyGroups(@AuthenticationPrincipal jwt: Jwt): List<GroupResponse> {
        val userId = UUID.fromString(jwt.subject)
        return groupService.listGroupsForUser(userId)
    }

    @GetMapping("/{id}")
    fun getGroup(@PathVariable id: UUID): GroupResponse {
        return groupService.getById(id)
    }

    // -------- MEMBERSHIP --------

    @PostMapping("/{groupId}/members")
    fun joinGroup(
        @AuthenticationPrincipal jwt: Jwt,
        @PathVariable groupId: UUID
    ) {
        val userId = UUID.fromString(jwt.subject)
        groupMemberService.joinGroup(userId, groupId)
    }

    @DeleteMapping("/{groupId}/members/me")
    fun leaveGroup(
        @AuthenticationPrincipal jwt: Jwt,
        @PathVariable groupId: UUID
    ) {
        val userId = UUID.fromString(jwt.subject)
        groupMemberService.leaveGroup(userId, groupId)
    }

    @GetMapping("/{groupId}/members")
    fun listMembers(
        @PathVariable groupId: UUID
    ): List<GroupMemberResponse> {
        return groupMemberService.listMembers(groupId)
    }
}
