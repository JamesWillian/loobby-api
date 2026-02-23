package com.jammes.loobby.users.service

import com.jammes.loobby.users.dto.UpdateUserProfileRequest
import com.jammes.loobby.users.dto.UserMeResponse
import com.jammes.loobby.users.dto.UserProfileResponse
import com.jammes.loobby.users.repo.UserCredentialsRepository
import com.jammes.loobby.users.repo.UsersRepository
import org.springframework.stereotype.Service
import org.springframework.transaction.annotation.Transactional
import java.util.UUID

@Service
class UsersService(
    private val usersRepository: UsersRepository,
    private val credentialsRepository: UserCredentialsRepository
) {

    fun getMe(userId: UUID): UserMeResponse {
        val user = usersRepository.findById(userId)
            .orElseThrow { IllegalArgumentException("User not found") }

        val credentials = credentialsRepository.findByUserId(userId)

        val roles = credentials?.roles ?: listOf("ANON")

        return UserMeResponse(
            id = user.id,
            username = user.username,
            displayname = user.displayname,
            avatarUrl = user.avatarUrl,
            isAnonymous = credentials == null,
            roles = roles,
            email = credentials?.email,
            createdAt = user.createdAt
        )
    }

    @Transactional
    fun updateUserProfile(userId: UUID, req: UpdateUserProfileRequest): UserProfileResponse {
        val user = usersRepository.findById(userId)
            .orElseThrow { IllegalArgumentException("User not found") }

        // Se veio username, valida e altera
        req.username?.let { newUsername ->
            val trimmedUsername = newUsername.trim()
            if (trimmedUsername.isBlank()) {
                throw IllegalArgumentException("Username cannot be blank")
            }

            // garantir que não está usando de outro usuário
            val exists = usersRepository.existsByUsernameIgnoreCase(trimmedUsername)
            if (exists && !trimmedUsername.equals(user.username, ignoreCase = true)) {
                throw IllegalArgumentException("Username is already taken")
            }

            user.username = trimmedUsername
        }

        // displayname pode ser nulo ou vazio → permite limpar
        req.displayname?.let { newDisplayname ->
            user.displayname = newDisplayname.trim().ifBlank { null }
        }

        // avatarUrl também pode ser null → remove avatar
        req.avatarUrl?.let { newAvatarUrl ->
            user.avatarUrl = newAvatarUrl.trim().ifBlank { null }
        }

        val saved = usersRepository.save(user)

        return UserProfileResponse(
            id = saved.id,
            username = saved.username,
            displayname = saved.displayname,
            avatarUrl = saved.avatarUrl,
            createdAt = saved.createdAt
        )
    }

    @Transactional
    fun updateAvatar(userId: UUID, avatarUrl: String): UserProfileResponse {
        val user = usersRepository.findById(userId)
            .orElseThrow { IllegalArgumentException("User not found") }

        user.avatarUrl = avatarUrl

        val saved = usersRepository.save(user)

        return UserProfileResponse(
            id = saved.id,
            username = saved.username,
            displayname = saved.displayname,
            avatarUrl = saved.avatarUrl,
            createdAt = saved.createdAt
        )
    }
}
