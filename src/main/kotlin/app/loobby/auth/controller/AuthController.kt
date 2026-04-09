package app.loobby.auth.controller

import app.loobby.auth.dto.AuthResponse
import app.loobby.auth.dto.LoginRequest
import app.loobby.auth.dto.RefreshTokenRequest
import app.loobby.auth.dto.RegisterRequest
import app.loobby.auth.service.AuthService
import app.loobby.auth.service.EmailVerificationService
import jakarta.validation.Valid
import org.springframework.http.ResponseEntity
import org.springframework.security.core.annotation.AuthenticationPrincipal
import org.springframework.security.oauth2.jwt.Jwt
import org.springframework.security.oauth2.jwt.JwtDecoder
import org.springframework.security.oauth2.jwt.JwtException
import org.springframework.web.bind.annotation.*
import org.springframework.web.server.ResponseStatusException
import java.util.UUID

@RestController
@RequestMapping("/auth")
class AuthController(
    private val authService: AuthService,
    private val emailVerificationService: EmailVerificationService,
    private val jwtDecoder: JwtDecoder
) {

    // -------------------------------
    // /auth/anonymous (público)
    // -------------------------------
    @PostMapping("/anonymous")
    fun createAnonymous(): AuthResponse {
        return authService.createAnonymous()
    }

    // -------------------------------
    // /auth/register (AUTENTICADO COMO ANÔNIMO)
    // -------------------------------
    @PostMapping("/register")
    fun register(
        @AuthenticationPrincipal jwt: Jwt,
        @Valid @RequestBody req: RegisterRequest
    ): AuthResponse {
        val userId = UUID.fromString(jwt.subject)
        val response = authService.register(userId, req)
        emailVerificationService.sendVerificationEmail(userId)
        return response
    }

    // -------------------------------
    // /auth/login (público)
    // -------------------------------
    @PostMapping("/login")
    fun login(@RequestBody req: LoginRequest): AuthResponse {
        return authService.login(req)
    }

    // -------------------------------
    // /auth/refresh (público)
    // se o refreshToken estiver expirado e o user for anônimo,
    // gera novos tokens automaticamente em vez de retornar 401.
    // -------------------------------
    @PostMapping("/refresh")
    fun refresh(@RequestBody req: RefreshTokenRequest): AuthResponse {
        return try {
            // Fluxo normal: token válido (não expirado)
            val jwt = jwtDecoder.decode(req.refreshToken)
            validateAndRefresh(jwt)
        } catch (ex: JwtException) {
            // Token expirado ou inválido — tenta recuperar anônimo
            authService.refreshExpiredAnonymousOrThrow(req.refreshToken, ex)
        }
    }

    /**
     * GET /auth/verify-email?token=XXX
     * Público — chamado quando o usuário clica no link do email.
     * Retorna uma página HTML simples de sucesso/erro.
     */
    @GetMapping("/auth/verify-email")
    fun verifyEmail(@RequestParam token: String): ResponseEntity<String> {
        return try {
            emailVerificationService.verifyEmail(token)
            ResponseEntity.ok()
                .header("Content-Type", "text/html")
                .body(buildSuccessHtml())
        } catch (e: ResponseStatusException) {
            ResponseEntity.status(e.statusCode)
                .header("Content-Type", "text/html")
                .body(buildErrorHtml(e.reason ?: "Erro na verificação"))
        }
    }

    /**
     * POST /auth/resend-verification
     * Autenticado — reenvia o email de verificação com rate limit.
     */
    @PostMapping("/auth/resend-verification")
    fun resendVerification(@AuthenticationPrincipal jwt: Jwt): ResponseEntity<Map<String, String>> {
        val userId = UUID.fromString(jwt.subject)
        emailVerificationService.resendVerificationEmail(userId)
        return ResponseEntity.ok(mapOf("message" to "Email de verificação reenviado"))
    }

    // ─── HTML helpers ────────────────────────────────────

    private fun buildSuccessHtml(): String = """
        <!DOCTYPE html>
        <html>
        <head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0">
        <title>Email Verificado</title></head>
        <body style="font-family: -apple-system, sans-serif; background: #0f1117; color: #e0e0e0; display: flex; justify-content: center; align-items: center; min-height: 100vh; margin: 0;">
            <div style="text-align: center; max-width: 400px; padding: 40px;">
                <div style="font-size: 64px; margin-bottom: 16px;">✅</div>
                <h1 style="color: #4ade80; margin-bottom: 12px;">Email verificado!</h1>
                <p style="color: #a0a0a0; line-height: 1.6;">Seu email foi confirmado com sucesso. Você já pode voltar ao app e aproveitar todos os recursos do Loobby.</p>
            </div>
        </body>
        </html>
    """.trimIndent()

    private fun buildErrorHtml(message: String): String = """
        <!DOCTYPE html>
        <html>
        <head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0">
        <title>Erro na Verificação</title></head>
        <body style="font-family: -apple-system, sans-serif; background: #0f1117; color: #e0e0e0; display: flex; justify-content: center; align-items: center; min-height: 100vh; margin: 0;">
            <div style="text-align: center; max-width: 400px; padding: 40px;">
                <div style="font-size: 64px; margin-bottom: 16px;">❌</div>
                <h1 style="color: #ef4444; margin-bottom: 12px;">Erro na verificação</h1>
                <p style="color: #a0a0a0; line-height: 1.6;">$message</p>
                <p style="color: #707070; font-size: 13px; margin-top: 24px;">Abra o app e solicite um novo email de verificação.</p>
            </div>
        </body>
        </html>
    """.trimIndent()

    /**
     * Valida claims do JWT já decodificado e gera novos tokens.
     */
    private fun validateAndRefresh(jwt: Jwt): AuthResponse {
        val type = jwt.claims["type"] as? String
            ?: throw IllegalArgumentException("Invalid token: missing type")

        if (type != "refresh") {
            throw IllegalArgumentException("Invalid token type")
        }

        val userId = UUID.fromString(jwt.subject)
        val user = authService.loadUserById(userId)

        return authService.generateAuthResponseForUser(user)
    }
}