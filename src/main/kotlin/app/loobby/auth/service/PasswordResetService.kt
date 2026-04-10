package app.loobby.auth.service

import app.loobby.users.repo.UserCredentialsRepository
import app.loobby.common.email.EmailService
import org.springframework.beans.factory.annotation.Value
import org.springframework.http.HttpStatus
import org.springframework.security.crypto.password.PasswordEncoder
import org.springframework.stereotype.Service
import org.springframework.transaction.annotation.Transactional
import org.springframework.web.server.ResponseStatusException
import java.time.OffsetDateTime
import java.util.*

@Service
class PasswordResetService(
    private val credentialRepository: UserCredentialsRepository,
    private val emailService: EmailService,
    private val passwordEncoder: PasswordEncoder,
    @Value("\${app.base-url:https://api.loobby.app}") private val baseUrl: String,
    @Value("\${app.password-reset.expiration-hours:1}") private val expirationHours: Long,
    @Value("\${app.password-reset.resend-cooldown-minutes:3}") private val cooldownMinutes: Long
) {

    /**
     * Gera token de reset e envia email.
     * NÃO revela se o email existe ou não (segurança).
     */
    @Transactional
    fun requestPasswordReset(email: String) {
        val credential = credentialRepository.findByEmail(email) ?: return
        // Silenciosamente ignora se email não existe — evita enumeration

        // Rate limit
        val expiresAt = credential.passwordResetExpiresAt
        if (expiresAt != null) {
            val tokenCreatedAt = expiresAt.minusHours(expirationHours)
            val cooldownEnd = tokenCreatedAt.plusMinutes(cooldownMinutes)
            if (OffsetDateTime.now().isBefore(cooldownEnd)) {
                return // Silenciosamente ignora — não revela que email existe
            }
        }

        val token = UUID.randomUUID().toString()
        credential.passwordResetToken = token
        credential.passwordResetExpiresAt = OffsetDateTime.now().plusHours(expirationHours)
        credentialRepository.save(credential)

        val resetUrl = "$baseUrl/auth/reset-password?token=$token"

        emailService.send(
            to = credential.email,
            subject = "Redefinir senha — Loobby",
            html = buildResetEmailHtml(resetUrl)
        )
    }

    /**
     * Valida se o token é válido (para exibir o formulário).
     * Retorna o email associado ou lança exceção.
     */
    fun validateResetToken(token: String): String {
        val credential = credentialRepository.findByPasswordResetToken(token)
            ?: throw ResponseStatusException(HttpStatus.BAD_REQUEST, "Link inválido ou já utilizado.")

        val expiresAt = credential.passwordResetExpiresAt
        if (expiresAt == null || OffsetDateTime.now().isAfter(expiresAt)) {
            throw ResponseStatusException(HttpStatus.BAD_REQUEST, "Link expirado. Solicite uma nova redefinição.")
        }

        return credential.email
    }

    /**
     * Redefine a senha usando o token.
     */
    @Transactional
    fun resetPassword(token: String, newPassword: String) {
        if (newPassword.length < 6) {
            throw ResponseStatusException(HttpStatus.BAD_REQUEST, "A senha deve ter pelo menos 6 caracteres.")
        }

        val credential = credentialRepository.findByPasswordResetToken(token)
            ?: throw ResponseStatusException(HttpStatus.BAD_REQUEST, "Link inválido ou já utilizado.")

        val expiresAt = credential.passwordResetExpiresAt
        if (expiresAt == null || OffsetDateTime.now().isAfter(expiresAt)) {
            throw ResponseStatusException(HttpStatus.BAD_REQUEST, "Link expirado. Solicite uma nova redefinição.")
        }

        credential.passwordHash = passwordEncoder.encode(newPassword).toString()
        credential.passwordResetToken = null
        credential.passwordResetExpiresAt = null
        credentialRepository.save(credential)
    }

    private fun buildResetEmailHtml(resetUrl: String): String = """
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
                
                <h2 style="color: #ffffff; font-size: 20px; margin-bottom: 16px;">Redefinir senha</h2>
                <p style="color: #c0c0c0; font-size: 15px; line-height: 1.6; margin-bottom: 32px;">
                    Recebemos uma solicitação para redefinir a senha da sua conta. Clique no botão abaixo para criar uma nova senha.
                </p>
                
                <a href="$resetUrl" 
                   style="display: inline-block; background-color: #4ade80; color: #0f1117; text-decoration: none; padding: 14px 40px; border-radius: 8px; font-weight: 600; font-size: 16px;">
                    Redefinir senha
                </a>
                
                <p style="color: #707070; font-size: 12px; margin-top: 32px; line-height: 1.5;">
                    Este link expira em 1 hora.<br>
                    Se você não solicitou a redefinição de senha, ignore este email.
                </p>
            </div>
        </body>
        </html>
    """.trimIndent()
}