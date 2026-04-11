package app.loobby.users.controller

import app.loobby.common.files.FileStorageService
import app.loobby.users.dto.ChangePasswordRequest
import app.loobby.users.dto.UpdateUserProfileRequest
import app.loobby.users.dto.UserFeedResponse
import app.loobby.users.dto.UserMeResponse
import app.loobby.users.dto.UserProfileResponse
import app.loobby.users.service.UsersService
import jakarta.validation.Valid
import org.springframework.http.ResponseEntity
import org.springframework.security.core.annotation.AuthenticationPrincipal
import org.springframework.security.oauth2.jwt.Jwt
import org.springframework.web.bind.annotation.GetMapping
import org.springframework.web.bind.annotation.PatchMapping
import org.springframework.web.bind.annotation.PostMapping
import org.springframework.web.bind.annotation.PutMapping
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

    @GetMapping("/feed")
    fun getUserFeed(
        @AuthenticationPrincipal jwt: Jwt
    ): List<UserFeedResponse> {
        val userId = UUID.fromString(jwt.subject)
        return usersService.getUserFeed(userId)
    }

    /**
     * PUT /users/me/password
     * Autenticado — altera a senha do usuário logado.
     */
    @PutMapping("/me/password")
    fun changePassword(
        @AuthenticationPrincipal jwt: Jwt,
        @RequestBody request: ChangePasswordRequest
    ): ResponseEntity<Map<String, String>> {
        val userId = UUID.fromString(jwt.subject)
        usersService.changePassword(userId, request)
        return ResponseEntity.ok(mapOf("message" to "Senha alterada com sucesso."))
    }
}
