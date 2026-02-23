package com.jammes.loobby.users.controller

import com.jammes.loobby.common.files.FileStorageService
import com.jammes.loobby.users.dto.UpdateUserProfileRequest
import com.jammes.loobby.users.dto.UserMeResponse
import com.jammes.loobby.users.dto.UserProfileResponse
import com.jammes.loobby.users.service.UsersService
import jakarta.validation.Valid
import org.springframework.security.core.annotation.AuthenticationPrincipal
import org.springframework.security.oauth2.jwt.Jwt
import org.springframework.web.bind.annotation.GetMapping
import org.springframework.web.bind.annotation.PatchMapping
import org.springframework.web.bind.annotation.PostMapping
import org.springframework.web.bind.annotation.RequestBody
import org.springframework.web.bind.annotation.RequestMapping
import org.springframework.web.bind.annotation.RequestParam
import org.springframework.web.bind.annotation.RestController
import org.springframework.web.multipart.MultipartFile
import java.util.UUID

@RestController
@RequestMapping("/users")
class UsersController(
    private val usersService: UsersService,
    private val fileStorageService: FileStorageService
) {

    @GetMapping("/me")
    fun getMe(@AuthenticationPrincipal jwt: Jwt): UserMeResponse {
        val userId = UUID.fromString(jwt.subject)
        return usersService.getMe(userId)
    }

    @PatchMapping("/me")
    fun updateMe(
        @AuthenticationPrincipal jwt: Jwt,
        @Valid @RequestBody req: UpdateUserProfileRequest
    ): UserProfileResponse {
        val userId = UUID.fromString(jwt.subject)
        return usersService.updateUserProfile(userId, req)
    }

    @PostMapping("/me/avatar")
    fun uploadAvatar(
        @AuthenticationPrincipal jwt: Jwt,
        @RequestParam("file") file: MultipartFile
    ): UserProfileResponse {
        val userId = UUID.fromString(jwt.subject)

        val avatarUrl = fileStorageService.store(
            file = file,
            subfolder = "avatars",
            ownerId = userId
        )

        return usersService.updateAvatar(userId, avatarUrl)
    }
}
