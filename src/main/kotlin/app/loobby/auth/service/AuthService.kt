package app.loobby.auth.service

import app.loobby.auth.dto.LoginRequest
import app.loobby.auth.dto.RegisterRequest
import app.loobby.auth.dto.AuthResponse
import app.loobby.config.security.JwtConfig
import app.loobby.users.model.UserEntity
import app.loobby.users.model.UserCredentialsEntity
import app.loobby.users.repo.UserCredentialsRepository
import app.loobby.users.repo.UsersRepository
import com.nimbusds.jwt.SignedJWT
import org.springframework.security.crypto.password.PasswordEncoder
import org.springframework.security.oauth2.jwt.JwtException
import org.springframework.stereotype.Service
import org.springframework.transaction.annotation.Transactional
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

        val roles = when (user.authProvider) {
            0    -> listOf("ANON")
            else -> listOf("USER")  // 1=EMAIL, 2=GOOGLE
        }

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

        // 3. Verifica se é anônimo
        if (user.authProvider != 0) throw originalException

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
    // @Transactional garante atomicidade entre o save de UserEntity e
    // UserCredentialsEntity — sem ele, se um falhasse o outro ficaria órfão.
    // Também garante que a mutação `user.authProvider = 1` seja persistida
    // (com `usersRepository.save(user)` explícito por clareza).
    @Transactional
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

        user.authProvider = 1
        usersRepository.save(user)
        credentialsRepository.save(creds)

        return generateAuthResponseForUser(user)
    }

    // --------------------------------------
    // LOGIN
    // --------------------------------------
    @Transactional
    fun login(request: LoginRequest): AuthResponse {
        val creds = credentialsRepository.findByEmail(request.email)
            ?: throw IllegalArgumentException("Invalid credentials")

        if (!passwordEncoder.matches(request.password, creds.passwordHash)) {
            throw IllegalArgumentException("Invalid credentials")
        }

        val user = usersRepository.findById(creds.userId)
            .orElseThrow { IllegalArgumentException("User not found") }

        // Self-heal: usuários registrados com a versão antiga de register()
        // (que nunca persistia user.authProvider = 1) ficaram com authProvider=0
        // no banco. Se chegamos aqui é porque existem credenciais EMAIL válidas
        // pra esse user — corrige o flag pra que /users/me devolva
        // isAnonymous=false na próxima chamada.
        if (user.authProvider == 0) {
            user.authProvider = 1
            usersRepository.save(user)
        }

        return generateAuthResponseForUser(user)
    }
}