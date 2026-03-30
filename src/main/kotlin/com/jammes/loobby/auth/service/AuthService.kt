package com.jammes.loobby.auth.service

import com.jammes.loobby.auth.dto.LoginRequest
import com.jammes.loobby.auth.dto.RegisterRequest
import com.jammes.loobby.auth.dto.AuthResponse
import com.jammes.loobby.config.security.JwtConfig
import com.jammes.loobby.users.model.UserEntity
import com.jammes.loobby.users.model.UserCredentialsEntity
import com.jammes.loobby.users.repo.UserCredentialsRepository
import com.jammes.loobby.users.repo.UsersRepository
import com.nimbusds.jwt.SignedJWT
import org.springframework.security.crypto.password.PasswordEncoder
import org.springframework.security.oauth2.jwt.JwtException
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

    // ──────────────────────────────────────────────────────
    // Tenta renovar tokens para anônimos com refresh expirado.
    // Se o user NÃO for anônimo, re-lança a exceção original (401).
    // ──────────────────────────────────────────────────────
    fun refreshExpiredAnonymousOrThrow(
        rawRefreshToken: String,
        originalException: JwtException
    ): AuthResponse {
        // 1. Parse manual do JWT (sem validar expiração) para extrair o subject
        val userId: UUID = try {
            val signedJWT = SignedJWT.parse(rawRefreshToken)
            val claims = signedJWT.jwtClaimsSet

            // Garante que é um refresh token (mesmo expirado, a claim "type" permanece)
            val type = claims.getStringClaim("type")
            if (type != "refresh") {
                throw originalException
            }

            UUID.fromString(claims.subject)
        } catch (ex: JwtException) {
            throw originalException
        } catch (ex: Exception) {
            // Parse falhou (token corrompido/adulterado) → mantém o erro original
            throw originalException
        }

        // 2. Verifica se o user existe
        val user = usersRepository.findById(userId).orElseThrow { originalException }

        // 3. Verifica se é anônimo (sem credentials no banco)
        val hasCredentials = credentialsRepository.existsByUserId(userId)
        if (hasCredentials) {
            // Não é anônimo → não pode renovar com token expirado
            throw originalException
        }

        // 4. É anônimo → gera novos tokens
        return generateAuthResponseForUser(user)
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