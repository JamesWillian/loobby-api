package app.loobby.auth.service

import app.loobby.auth.dto.AuthResponse
import app.loobby.common.phone.PhoneNormalizer
import app.loobby.common.phone.PhoneVerificationException
import app.loobby.common.phone.PhoneVerifier
import app.loobby.users.model.UserEntity
import app.loobby.users.model.UserGoogleCredentialsEntity
import app.loobby.users.repo.UserGoogleCredentialsRepository
import app.loobby.users.repo.UsersRepository
import app.loobby.users.service.PhoneAttachOutcome
import app.loobby.users.service.UserMergeService
import com.google.api.client.googleapis.auth.oauth2.GoogleIdToken
import com.google.api.client.googleapis.auth.oauth2.GoogleIdTokenVerifier
import com.google.api.client.http.javanet.NetHttpTransport
import com.google.api.client.json.gson.GsonFactory
import org.springframework.beans.factory.annotation.Value
import org.springframework.http.HttpStatus
import org.springframework.stereotype.Service
import org.springframework.transaction.annotation.Transactional
import org.springframework.web.server.ResponseStatusException
import java.util.UUID

@Service
class GoogleAuthService(
    private val authService: AuthService,
    private val usersRepository: UsersRepository,
    private val googleCredentialsRepository: UserGoogleCredentialsRepository,
    private val phoneVerifier: PhoneVerifier,
    private val userMergeService: UserMergeService,
    @Value("\${google.client-ids}") private val googleClientIds: List<String>
) {

    /**
     * Login ou cadastro via conta Google.
     *
     * @param idToken ID token do Google Sign-In (obrigatório).
     * @param firebaseIdToken ID token do Firebase Phone Auth (opcional durante o
     *        rollout do Android com telefone obrigatório). Quando presente, o
     *        telefone é vinculado ao usuário e um eventual lite com o mesmo
     *        telefone é mesclado. Se o telefone já pertencer a outra conta
     *        full, o request é rejeitado com 409.
     *
     * @Transactional garante que, se a vinculação de telefone falhar com 409,
     * a criação de um novo UserEntity/UserGoogleCredentialsEntity seja revertida
     * — sem isso, ficaríamos com um usuário "fantasma" no banco a cada tentativa
     * de signup conflitando com telefone existente.
     */
    @Transactional
    fun loginOrRegister(idToken: String, firebaseIdToken: String? = null): AuthResponse {
        val payload = verifyGoogleToken(idToken)

        val googleId  = payload.subject
        val email     = payload.email ?: throw ResponseStatusException(HttpStatus.BAD_REQUEST, "Email não disponível")
        val name      = payload["name"] as? String
        val photoUrl  = payload["picture"] as? String

        val user = findOrCreateUserForGoogle(googleId, email, name, photoUrl)

        // Vincula/mescla telefone, se fornecido.
        attachPhoneIfProvided(user.id, firebaseIdToken)

        return authService.generateAuthResponseForUser(user)
    }

    // -------------------------------------------------------------------------
    // Internals
    // -------------------------------------------------------------------------

    private fun findOrCreateUserForGoogle(
        googleId: String,
        email: String,
        name: String?,
        photoUrl: String?
    ): UserEntity {
        val existing = googleCredentialsRepository.findByGoogleId(googleId)

        if (existing != null) {
            // Login: usuário já existe.
            val u = usersRepository.findById(existing.userId)
                .orElseThrow { ResponseStatusException(HttpStatus.NOT_FOUND, "User not found") }

            // Atualiza avatarUrl se o Google fornecer e o campo estiver vazio.
            if (photoUrl != null && u.avatarUrl.isNullOrBlank()) {
                u.avatarUrl = photoUrl
                usersRepository.save(u)
            }
            return u
        }

        // Registro: cria usuário + credenciais Google.
        val displayname = name ?: email.substringBefore("@")
        val newUser = usersRepository.save(
            UserEntity(
                id           = UUID.randomUUID(),
                username     = generateUsername(displayname),
                displayname  = displayname,
                avatarUrl    = photoUrl,
                authProvider = 2 // GOOGLE
            )
        )
        googleCredentialsRepository.save(
            UserGoogleCredentialsEntity(
                userId   = newUser.id,
                googleId = googleId,
                email    = email
            )
        )
        return newUser
    }

    /**
     * Verifica o ID token do Firebase Phone Auth (se fornecido) e vincula o
     * telefone ao usuário. Veja [AuthService.attachPhoneIfProvided] — esta é
     * a contraparte para o fluxo Google e segue exatamente o mesmo contrato.
     */
    private fun attachPhoneIfProvided(userId: UUID, firebaseIdToken: String?) {
        if (firebaseIdToken.isNullOrBlank()) return

        val verifiedPhone = try {
            phoneVerifier.verify(firebaseIdToken)
        } catch (e: PhoneVerificationException) {
            throw ResponseStatusException(HttpStatus.UNAUTHORIZED, e.message)
        }

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

    private fun verifyGoogleToken(idToken: String): GoogleIdToken.Payload {
        val verifier = GoogleIdTokenVerifier.Builder(NetHttpTransport(), GsonFactory())
            .setAudience(googleClientIds)
            .build()

        return verifier.verify(idToken)?.payload
            ?: throw ResponseStatusException(HttpStatus.UNAUTHORIZED, "Token do Google inválido")
    }

    private fun generateUsername(name: String): String {
        val base = name.lowercase()
            .replace(" ", "_")
            .filter { it.isLetterOrDigit() || it == '_' }
            .take(12)
        return "${base}_${UUID.randomUUID().toString().take(4)}"
    }
}
