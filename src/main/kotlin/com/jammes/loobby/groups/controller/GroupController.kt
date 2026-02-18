package com.jammes.loobby.groups.controller

import com.jammes.loobby.groups.dto.CreateGroupRequest
import com.jammes.loobby.groups.dto.GroupResponse
import com.jammes.loobby.groups.service.GroupService
import org.springframework.security.core.annotation.AuthenticationPrincipal
import org.springframework.security.oauth2.jwt.Jwt
import org.springframework.web.bind.annotation.*
import java.util.UUID
import jakarta.validation.Valid

@RestController
@RequestMapping("/groups")
class GroupController(
    private val groupService: GroupService
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
}
