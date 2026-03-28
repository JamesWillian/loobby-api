package com.jammes.loobby.groups.controller

import com.jammes.loobby.groups.dto.CreateGroupRequest
import com.jammes.loobby.groups.dto.GroupMemberResponse
import com.jammes.loobby.groups.dto.GroupResponse
import com.jammes.loobby.groups.dto.UpdateGroupRequest
import com.jammes.loobby.groups.service.GroupMemberService
import com.jammes.loobby.groups.service.GroupService
import org.springframework.security.core.annotation.AuthenticationPrincipal
import org.springframework.security.oauth2.jwt.Jwt
import org.springframework.web.bind.annotation.*
import java.util.UUID
import jakarta.validation.Valid
import org.springframework.http.HttpStatus
import org.springframework.web.multipart.MultipartFile

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

    @GetMapping("/invite/{code}")
    fun getByCode(@PathVariable code: String): GroupResponse {
        return groupService.getByCode(code.uppercase())
    }

    // -------- UPDATE GROUP NAME --------

    @PatchMapping("/{groupId}")
    fun updateGroup(
        @AuthenticationPrincipal jwt: Jwt,
        @PathVariable groupId: UUID,
        @Valid @RequestBody req: UpdateGroupRequest
    ): GroupResponse {
        val userId = UUID.fromString(jwt.subject)
        return groupService.updateGroupName(userId, groupId, req)
    }

    // -------- UPDATE GROUP IMAGE --------

    @PostMapping("/{groupId}/image")
    fun uploadGroupImage(
        @AuthenticationPrincipal jwt: Jwt,
        @PathVariable groupId: UUID,
        @RequestParam("file") file: MultipartFile
    ): GroupResponse {
        val userId = UUID.fromString(jwt.subject)
        return groupService.updateGroupImage(userId, groupId, file)
    }

    // -------- DELETE GROUP --------

    @DeleteMapping("/{groupId}")
    @ResponseStatus(HttpStatus.NO_CONTENT)
    fun deleteGroup(
        @AuthenticationPrincipal jwt: Jwt,
        @PathVariable groupId: UUID
    ) {
        val userId = UUID.fromString(jwt.subject)
        groupService.deleteGroup(userId, groupId)
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

    @DeleteMapping("/{groupId}/members/{memberId}")
    @ResponseStatus(HttpStatus.NO_CONTENT)
    fun removeMember(
        @AuthenticationPrincipal jwt: Jwt,
        @PathVariable groupId: UUID,
        @PathVariable memberId: UUID
    ) {
        val userId = UUID.fromString(jwt.subject)
        groupMemberService.removeMember(userId, groupId, memberId)
    }
}
