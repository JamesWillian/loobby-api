package com.jammes.loobby.users.service

import com.jammes.loobby.users.dto.UserMeResponse
import com.jammes.loobby.users.repo.UserCredentialsRepository
import com.jammes.loobby.users.repo.UsersRepository
import org.springframework.stereotype.Service
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
}
