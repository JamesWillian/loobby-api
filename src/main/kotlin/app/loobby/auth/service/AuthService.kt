package app.loobby.auth.service

import app.loobby.auth.dto.LoginRequest
import app.loobby.auth.dto.RegisterRequest
import app.loobby.auth.dto.AuthResponse
import app.loobby.common.phone.PhoneNormalizer
import app.loobby.common.phone.PhoneVerificationException
import app.loobby.common.phone.PhoneVerifier
import app.loobby.config.security.JwtConfig
import app.loobby.users.model.UserEntity
import app.loobby.users.model.UserCredentialsEntity
import app.loobby.users.repo.UserCredentialsRepository
import app.loobby.users.repo.UsersRepository
import app.loobby.users.service.PhoneAttachOutcome
import app.loobby.users.service.UserMergeService
import com.nimbusds.jwt.SignedJWT
import org.springframework.http.HttpStatus
import org.springframework.security.crypto.password.PasswordEncoder
import org.springframework.security.oauth2.jwt.JwtException
import org.springframework.stereotype.Service
import org.springframework.transaction.annotation.Transactional
import org.springframework.web.server.ResponseStatusException
import java.util.UUID

@Service
class AuthService(
    private val usersRepository: UsersRepository,
    private val credentialsRepository: UserCredentialsRepository,
    private val passwordEncoder: PasswordEncoder,
    private val tokenService: TokenService,
    private val jwtConfig: JwtConfig,
    private val phoneVerifier: PhoneVerifier,
    private val userMergeService: UserMergeService
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

        // Normaliza o email (trim + lowercase) para evitar duplicatas por
        // diferença de caixa/espaço e mantém consistência com o forgot-password.
        val normalizedEmail = request.email.trim().lowercase()

        // Verifica se o email já está em uso por outro usuário.
        // Sem essa checagem, o constraint UNIQUE do banco lança
        // DataIntegrityViolationException, que cai no handler genérico
        // e vira "Unexpected error" no mobile.
        if (credentialsRepository.existsByEmail(normalizedEmail)) {
            throw IllegalStateException("Email already registered")
        }

        val creds = UserCredentialsEntity(
            userId = userId,
            email = normalizedEmail,
            passwordHash = passwordEncoder.encode(request.password)!!,
        ).apply { setRoles(listOf("USER")) }

        user.authProvider = 1
        usersRepository.save(user)
        credentialsRepository.save(creds)

        // Se o app enviou um Firebase ID token do telefone, vinculamos o número
        // e (opcionalmente) mesclamos uma conta lite preexistente. Quando o token
        // é ausente, mantemos compatibilidade com versões antigas do mobile que
        // ainda não exigem telefone. A regra de bloqueio para telefone já
        // pertencente a outra conta full vive em [UserMergeService].
        attachPhoneIfProvided(user.id, request.firebaseIdToken)

        return generateAuthResponseForUser(user)
    }

    /**
     * Verifica o ID token (se fornecido), normaliza o telefone para garantir
     * formato E.164 e delega ao [UserMergeService] o anexo + eventual merge.
     *
     * Esta operação roda dentro da @Transactional do chamador — se o merge for
     * rejeitado por colisão com conta full, lançamos 409 e a transação inteira
     * de registro é revertida (evita usuário órfão sem credenciais coerentes).
     */
    private fun attachPhoneIfProvided(userId: UUID, firebaseIdToken: String?) {
        if (firebaseIdToken.isNullOrBlank()) return

        val verifiedPhone = try {
            phoneVerifier.verify(firebaseIdToken)
        } catch (e: PhoneVerificationException) {
            throw ResponseStatusException(HttpStatus.UNAUTHORIZED, e.message)
        }

        // O Firebase devolve o phone já em E.164; renormalizar é defensivo e
        // garante invariante mesmo se algum provedor futuro devolver formato
        // diferente.
        val normalized = try {
            PhoneNormalizer.toE164(verifiedPhone)
        } catch (e: IllegalArgumentException) {
            throw ResponseStatusException(HttpStatus.BAD_REQUEST, "phone in token is not a valid E.164 number")
        }

        when (userMergeService.attachPhoneAndMergeLite(userId, normalized)) {
            PhoneAttachOutcome.REJECTED_FULL_ACCOUNT ->
                throw ResponseStatusException(
                    HttpStatus.CONFLICT,
                    "Esse telefone já está vinculado a outra conta. Faça login na conta existente."
                )
            PhoneAttachOutcome.MERGED,
            PhoneAttachOutcome.PHONE_ATTACHED -> { /* segue o fluxo */ }
        }
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