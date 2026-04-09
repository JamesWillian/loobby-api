package app.loobby.auth.service

import app.loobby.common.email.EmailService
import app.loobby.users.repo.UserCredentialsRepository
import org.springframework.beans.factory.annotation.Value
import org.springframework.http.HttpStatus
import org.springframework.stereotype.Service
import org.springframework.transaction.annotation.Transactional
import org.springframework.web.server.ResponseStatusException
import java.time.OffsetDateTime
import java.util.*

@Service
class EmailVerificationService(
    private val credentialRepository: UserCredentialsRepository,
    private val emailService: EmailService,
    @Value("\${app.base-url}") private val baseUrl: String,
    @Value("\${app.email-verification.expiration-hours}") private val expirationHours: Long,
    @Value("\${app.email-verification.resend-cooldown-minutes}") private val resendCooldownMinutes: Long
) {

    /**
     * Gera token de verificação e envia o email.
     * Chamado após o registro.
     */
    @Transactional
    fun sendVerificationEmail(userId: UUID) {
        val credential = credentialRepository.findById(userId)
            .orElseThrow { ResponseStatusException(HttpStatus.NOT_FOUND, "Usuário não encontrado") }

        if (credential.emailVerified) {
            return // Já verificado, não faz nada
        }

        val token = UUID.randomUUID().toString()
        credential.emailVerificationToken = token
        credential.emailVerificationExpiresAt = OffsetDateTime.now().plusHours(expirationHours)
        credentialRepository.save(credential)

        val verifyUrl = "$baseUrl/auth/verify-email?token=$token"

        emailService.send(
            to = credential.email,
            subject = "Confirme seu email no Loobby",
            html = buildVerificationEmailHtml(verifyUrl)
        )
    }

    /**
     * Verifica o token recebido via link do email.
     * Retorna true se verificação foi bem sucedida.
     */
    @Transactional
    fun verifyEmail(token: String): Boolean {
        val credential = credentialRepository.findByEmailVerificationToken(token)
            ?: throw ResponseStatusException(HttpStatus.BAD_REQUEST, "Token inválido ou já utilizado")

        if (credential.emailVerified) {
            return true // Já verificado
        }

        val expiresAt = credential.emailVerificationExpiresAt
        if (expiresAt == null || OffsetDateTime.now().isAfter(expiresAt)) {
            throw ResponseStatusException(HttpStatus.BAD_REQUEST, "Token expirado. Solicite um novo email de verificação.")
        }

        credential.emailVerified = true
        credential.emailVerificationToken = null
        credential.emailVerificationExpiresAt = null
        credentialRepository.save(credential)

        return true
    }

    /**
     * Reenvia o email de verificação com rate limit.
     */
    @Transactional
    fun resendVerificationEmail(userId: UUID) {
        val credential = credentialRepository.findById(userId)
            .orElseThrow { ResponseStatusException(HttpStatus.NOT_FOUND, "Usuário não encontrado") }

        if (credential.emailVerified) {
            throw ResponseStatusException(HttpStatus.BAD_REQUEST, "Email já verificado")
        }

        // Rate limit: só permite reenviar após cooldown
        val expiresAt = credential.emailVerificationExpiresAt
        if (expiresAt != null) {
            val tokenCreatedAt = expiresAt.minusHours(expirationHours)
            val cooldownEnd = tokenCreatedAt.plusMinutes(resendCooldownMinutes)
            if (OffsetDateTime.now().isBefore(cooldownEnd)) {
                throw ResponseStatusException(
                    HttpStatus.TOO_MANY_REQUESTS,
                    "Aguarde alguns minutos antes de solicitar um novo email."
                )
            }
        }

        sendVerificationEmail(userId)
    }

    /**
     * Verifica se o userId tem email verificado.
     */
    fun isEmailVerified(userId: UUID): Boolean {
        val credential = credentialRepository.findById(userId).orElse(null)
        return credential?.emailVerified ?: false
    }

    private fun buildVerificationEmailHtml(verifyUrl: String): String = """
        <!DOCTYPE html>
        <html>
        <head>
            <meta charset="UTF-8">
            <meta name="viewport" content="width=device-width, initial-scale=1.0">
        </head>
        <body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background-color: #0f1117; color: #e0e0e0; padding: 40px 20px; margin: 0;">
            <div style="max-width: 480px; margin: 0 auto; background-color: #1a1d27; border-radius: 16px; padding: 40px; text-align: center;">
                <h1 style="color: #4ade80; font-size: 28px; margin-bottom: 8px;">Loobby</h1>
                <p style="color: #a0a0a0; font-size: 14px; margin-bottom: 32px;">Organização de eventos entre amigos</p>
                
                <h2 style="color: #ffffff; font-size: 20px; margin-bottom: 16px;">Confirme seu email</h2>
                <p style="color: #c0c0c0; font-size: 15px; line-height: 1.6; margin-bottom: 32px;">
                    Clique no botão abaixo para verificar seu endereço de email e desbloquear todos os recursos do Loobby.
                </p>
                
                <a href="$verifyUrl" 
                   style="display: inline-block; background-color: #4ade80; color: #0f1117; text-decoration: none; padding: 14px 40px; border-radius: 8px; font-weight: 600; font-size: 16px;">
                    Confirmar email
                </a>
                
                <p style="color: #707070; font-size: 12px; margin-top: 32px; line-height: 1.5;">
                    Este link expira em 24 horas.<br>
                    Se você não criou uma conta no Loobby, ignore este email.
                </p>
            </div>
        </body>
        </html>
    """.trimIndent()
}