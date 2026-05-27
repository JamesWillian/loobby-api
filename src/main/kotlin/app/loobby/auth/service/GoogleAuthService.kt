package app.loobby.auth.service

import app.loobby.auth.dto.AuthResponse
import app.loobby.users.model.UserEntity
import app.loobby.users.model.UserGoogleCredentialsEntity
import app.loobby.users.repo.UserGoogleCredentialsRepository
import app.loobby.users.repo.UsersRepository
import com.google.api.client.googleapis.auth.oauth2.GoogleIdToken
import com.google.api.client.googleapis.auth.oauth2.GoogleIdTokenVerifier
import com.google.api.client.http.javanet.NetHttpTransport
import com.google.api.client.json.gson.GsonFactory
import org.springframework.beans.factory.annotation.Value
import org.springframework.http.HttpStatus
import org.springframework.stereotype.Service
import org.springframework.web.server.ResponseStatusException
import java.util.UUID

@Service
class GoogleAuthService(
    private val authService: AuthService,
    private val usersRepository: UsersRepository,
    private val googleCredentialsRepository: UserGoogleCredentialsRepository,
    @Value("\${google.client-ids}") private val googleClientIds: List<String>
) {

    fun loginOrRegister(idToken: String): AuthResponse {
        val payload = verifyToken(idToken)

        val googleId  = payload.subject
        val email     = payload.email ?: throw ResponseStatusException(HttpStatus.BAD_REQUEST, "Email não disponível")
        val name      = payload["name"] as? String
        val photoUrl  = payload["picture"] as? String

        val existing = googleCredentialsRepository.findByGoogleId(googleId)

        val user: UserEntity = if (existing != null) {
            // ── Login: usuário já existe ──────────────────────────
            val u = usersRepository.findById(existing.userId)
                .orElseThrow { ResponseStatusException(HttpStatus.NOT_FOUND, "User not found") }

            // Atualiza avatarUrl se o Google fornecer e o campo estiver vazio
            var updated = false
            if (photoUrl != null && u.avatarUrl.isNullOrBlank()) {
                u.avatarUrl = photoUrl
                updated = true
            }
            if (updated) usersRepository.save(u)
            u

        } else {
            // ── Registro: cria usuário + credenciais Google ───────
            val displayname = name ?: email.substringBefore("@")
            val newUser = usersRepository.save(
                UserEntity(
                    id           = UUID.randomUUID(),
                    username     = generateUsername(displayname),
                    displayname  = displayname,   // aproveita nome do Google
                    avatarUrl    = photoUrl,      // aproveita foto do Google
                    authProvider = 2              // GOOGLE
                )
            )
            googleCredentialsRepository.save(
                UserGoogleCredentialsEntity(
                    userId   = newUser.id,
                    googleId = googleId,
                    email    = email
                )
            )
            newUser
        }

        return authService.generateAuthResponseForUser(user)
    }

    private fun verifyToken(idToken: String): GoogleIdToken.Payload {
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