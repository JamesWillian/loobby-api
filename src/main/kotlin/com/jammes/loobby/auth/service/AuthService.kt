package com.jammes.loobby.auth.service

import com.jammes.loobby.auth.dto.LoginRequest
import com.jammes.loobby.auth.dto.RegisterRequest
import com.jammes.loobby.auth.dto.AuthResponse
import com.jammes.loobby.config.security.JwtConfig
import com.jammes.loobby.users.model.UserEntity
import com.jammes.loobby.users.model.UserCredentialsEntity
import com.jammes.loobby.users.repo.UserCredentialsRepository
import com.jammes.loobby.users.repo.UsersRepository
import org.springframework.security.crypto.password.PasswordEncoder
import org.springframework.stereotype.Service
import java.util.UUID

@Service
class AuthService(
    private val usersRepository: UsersRepository,
    private val credentialsRepository: UserCredentialsRepository,
    private val passwordEncoder: PasswordEncoder,
    private val tokenService: TokenService,
    private val jwtConfig: JwtConfig
) {

    fun loadUserById(userId: UUID): UserEntity {
        return usersRepository.findById(userId)
            .orElseThrow { IllegalArgumentException("User not found") }
    }

    fun generateAuthResponseForUser(user: UserEntity): AuthResponse {

        val credentials = credentialsRepository.findByUserId(user.id)

        val roles = credentials?.roles ?: listOf("ANON")

        val accessToken = tokenService.generateAccessToken(
            userId = user.id,
            username = user.username,
            roles = roles
        )

        // gera um novo refreshToken
        val refreshToken = tokenService.generateRefreshToken(user.id)

        return AuthResponse(
            accessToken = accessToken,
            refreshToken = refreshToken,
            expiresIn = jwtConfig.accessTokenValidityMinutes * 60,
            userId = user.id,
            username = user.username,
            roles = roles
        )
    }

    // --------------------------------------
    // CRIA USUÁRIO ANÔNIMO
    // --------------------------------------
    fun createAnonymous(): AuthResponse {
        val username = generateRandomUsername()
        val user = usersRepository.save(
            UserEntity(
                id = UUID.randomUUID(),
                username = username,
                displayname = username,
                avatarUrl = null
            )
        )

        return generateAuthResponseForUser(user)
    }

    private fun generateRandomUsername(): String {
        return "user_" + UUID.randomUUID().toString().take(6)
    }

    // --------------------------------------
    // REGISTRO (UPGRADE DE ANÔNIMO)
    // --------------------------------------
    fun register(userId: UUID, request: RegisterRequest): AuthResponse {

        val user = usersRepository.findById(userId)
            .orElseThrow { IllegalArgumentException("User not found") }

        if (credentialsRepository.existsByUserId(userId)) {
            throw IllegalStateException("User already registered")
        }

        if (request.password.isEmpty()) {
            throw IllegalStateException("Password invalid")
        }

        val creds = UserCredentialsEntity(
            userId = userId,
            email = request.email,
            passwordHash = passwordEncoder.encode(request.password)!!,
        ).apply { setRoles(listOf("USER")) }

        credentialsRepository.save(creds)

        return generateAuthResponseForUser(user)
    }

    // --------------------------------------
    // LOGIN
    // --------------------------------------
    fun login(request: LoginRequest): AuthResponse {
        val creds = credentialsRepository.findByEmail(request.email)
            ?: throw IllegalArgumentException("Invalid credentials")

        if (!passwordEncoder.matches(request.password, creds.passwordHash)) {
            throw IllegalArgumentException("Invalid credentials")
        }

        val user = usersRepository.findById(creds.userId)
            .orElseThrow { IllegalArgumentException("User not found") }

        return generateAuthResponseForUser(user)
    }
}
