package app.loobby.users.service

import com.fasterxml.jackson.annotation.JsonProperty
import app.loobby.users.dto.UpdateUserProfileRequest
import app.loobby.users.dto.UserFeedResponse
import app.loobby.users.dto.UserMeResponse
import app.loobby.users.dto.UserProfileResponse
import app.loobby.users.repo.UserCredentialsRepository
import app.loobby.users.repo.UserFeedRepository
import app.loobby.users.repo.UsersRepository
import org.springframework.stereotype.Service
import org.springframework.transaction.annotation.Transactional
import java.util.UUID
import kotlin.String

@Service
class UsersService(
    private val usersRepository: UsersRepository,
    private val credentialsRepository: UserCredentialsRepository,
    private val userFeedRepository: UserFeedRepository
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
            emailVerified = credentials?.emailVerified ?: false,
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

    fun getUserFeed(userId: UUID): List<UserFeedResponse> {
//        val user = usersRepository.findById(userId)
//            .orElseThrow { IllegalArgumentException("User not found") }

        val feed = userFeedRepository.findByUserId(userId)

        return feed.map {
            UserFeedResponse(
                id = it.id,
                userId = it.userId,
                name = it.name,
                imageUrl = it.imageUrl,
                entryType = it.entryType,
                isFinished = it.isFinished
            )
        }
    }
}
