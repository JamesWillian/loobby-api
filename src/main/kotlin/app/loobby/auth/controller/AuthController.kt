package app.loobby.auth.controller

import app.loobby.auth.dto.AuthResponse
import app.loobby.auth.dto.ForgotPasswordRequest
import app.loobby.auth.dto.GoogleAuthRequest
import app.loobby.auth.dto.LoginRequest
import app.loobby.auth.dto.RefreshTokenRequest
import app.loobby.auth.dto.RegisterRequest
import app.loobby.auth.dto.ResetPasswordFormRequest
import app.loobby.auth.service.AuthService
import app.loobby.auth.service.EmailVerificationService
import app.loobby.auth.service.GoogleAuthService
import app.loobby.auth.service.PasswordResetService
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
    private val passwordResetService: PasswordResetService,
    private val googleAuthService: GoogleAuthService,
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
        val authResponse = authService.register(userId, req)
        emailVerificationService.sendVerificationEmail(userId)
        return authResponse
    }

    // -------------------------------
    // /auth/login (público)
    // -------------------------------
    @PostMapping("/login")
    fun login(@RequestBody req: LoginRequest): AuthResponse {
        val authResponse = authService.login(req)
        if (!emailVerificationService.isEmailVerified(authResponse.userId))
            emailVerificationService.sendVerificationEmail(authResponse.userId)
        return authResponse
    }

    // -------------------------------
    // /auth/google (público)
    // -------------------------------
    @PostMapping("/google")
    fun loginWithGoogle(@RequestBody req: GoogleAuthRequest): AuthResponse {
        return googleAuthService.loginOrRegister(req.idToken)
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
    @GetMapping("/verify-email")
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
    @PostMapping("/resend-verification")
    fun resendVerification(@AuthenticationPrincipal jwt: Jwt): ResponseEntity<Map<String, String>> {
        val userId = UUID.fromString(jwt.subject)
        emailVerificationService.resendVerificationEmail(userId)
        return ResponseEntity.ok(mapOf("message" to "Email de verificação reenviado"))
    }

    /**
     * POST /auth/forgot-password
     * Público — recebe email e envia link de reset.
     * Sempre retorna 200 (não revela se email existe).
     */
    @PostMapping("/forgot-password")
    fun forgotPassword(@RequestBody request: ForgotPasswordRequest): ResponseEntity<Map<String, String>> {
        passwordResetService.requestPasswordReset(request.email.trim().lowercase())
        return ResponseEntity.ok(
            mapOf("message" to "Se o email estiver cadastrado, você receberá um link para redefinir a senha.")
        )
    }

    /**
     * GET /auth/reset-password?token=XXX
     * Público — exibe formulário HTML para redefinir senha.
     */
    @GetMapping("/reset-password")
    fun showResetPasswordForm(@RequestParam token: String): ResponseEntity<String> {
        return try {
            val email = passwordResetService.validateResetToken(token)
            ResponseEntity.ok()
                .header("Content-Type", "text/html")
                .body(buildResetPasswordFormHtml(token, email))
        } catch (e: ResponseStatusException) {
            ResponseEntity.status(e.statusCode)
                .header("Content-Type", "text/html")
                .body(buildResetErrorHtml(e.reason ?: "Erro na redefinição"))
        }
    }

    /**
     * POST /auth/reset-password
     * Público — recebe token + nova senha do formulário HTML.
     */
    @PostMapping("/reset-password")
    fun resetPassword(@RequestBody request: ResetPasswordFormRequest): ResponseEntity<String> {
        return try {
            passwordResetService.resetPassword(request.token, request.password)
            ResponseEntity.ok()
                .header("Content-Type", "text/html")
                .body(buildResetSuccessHtml())
        } catch (e: ResponseStatusException) {
            ResponseEntity.status(e.statusCode)
                .header("Content-Type", "text/html")
                .body(buildResetErrorHtml(e.reason ?: "Erro na redefinição"))
        }
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

    private fun buildResetPasswordFormHtml(token: String, email: String): String = """
        <!DOCTYPE html>
        <html>
        <head>
            <meta charset="UTF-8">
            <meta name="viewport" content="width=device-width, initial-scale=1.0">
            <title>Redefinir Senha — Loobby</title>
            <style>
                * { box-sizing: border-box; margin: 0; padding: 0; }
                body {
                    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
                    background: #0f1117;
                    color: #e0e0e0;
                    display: flex;
                    justify-content: center;
                    align-items: center;
                    min-height: 100vh;
                    padding: 20px;
                }
                .card {
                    background: #1a1d27;
                    border-radius: 16px;
                    padding: 40px;
                    max-width: 420px;
                    width: 100%;
                    text-align: center;
                }
                h1 { color: #4ade80; font-size: 28px; margin-bottom: 8px; }
                .subtitle { color: #a0a0a0; font-size: 14px; margin-bottom: 24px; }
                h2 { color: #fff; font-size: 20px; margin-bottom: 8px; }
                .email-hint { color: #a0a0a0; font-size: 13px; margin-bottom: 24px; }
                .field {
                    width: 100%;
                    padding: 14px 16px;
                    border: 1px solid #333;
                    border-radius: 8px;
                    background: #0f1117;
                    color: #e0e0e0;
                    font-size: 15px;
                    margin-bottom: 12px;
                    outline: none;
                }
                .field:focus { border-color: #4ade80; }
                .btn {
                    width: 100%;
                    padding: 14px;
                    border: none;
                    border-radius: 8px;
                    background: #4ade80;
                    color: #0f1117;
                    font-size: 16px;
                    font-weight: 600;
                    cursor: pointer;
                    margin-top: 8px;
                }
                .btn:hover { background: #3bc96e; }
                .btn:disabled { background: #2a2d37; color: #707070; cursor: not-allowed; }
                .error { color: #ef4444; font-size: 13px; margin-top: 8px; display: none; }
                .hint { color: #707070; font-size: 12px; margin-top: 16px; }
            </style>
        </head>
        <body>
            <div class="card">
                <h1>Loobby</h1>
                <p class="subtitle">Organização de eventos entre amigos</p>
                <h2>Nova senha</h2>
                <p class="email-hint">Para a conta: <strong>$email</strong></p>
 
                <input type="password" id="password" class="field" placeholder="Nova senha (mín. 6 caracteres)" />
                <input type="password" id="confirmPassword" class="field" placeholder="Confirmar nova senha" />
                <p id="error" class="error"></p>
                <button id="submitBtn" class="btn" onclick="submitReset()">Redefinir senha</button>
                <p class="hint">A senha deve ter pelo menos 6 caracteres.</p>
            </div>
 
            <script>
                async function submitReset() {
                    const password = document.getElementById('password').value;
                    const confirm = document.getElementById('confirmPassword').value;
                    const errorEl = document.getElementById('error');
                    const btn = document.getElementById('submitBtn');
 
                    errorEl.style.display = 'none';
 
                    if (password.length < 6) {
                        errorEl.textContent = 'A senha deve ter pelo menos 6 caracteres.';
                        errorEl.style.display = 'block';
                        return;
                    }
                    if (password !== confirm) {
                        errorEl.textContent = 'As senhas não coincidem.';
                        errorEl.style.display = 'block';
                        return;
                    }
 
                    btn.disabled = true;
                    btn.textContent = 'Redefinindo...';
 
                    try {
                        const res = await fetch('/auth/reset-password', {
                            method: 'POST',
                            headers: { 'Content-Type': 'application/json' },
                            body: JSON.stringify({ token: '$token', password: password })
                        });
                        const html = await res.text();
                        document.open();
                        document.write(html);
                        document.close();
                    } catch (e) {
                        errorEl.textContent = 'Erro de conexão. Tente novamente.';
                        errorEl.style.display = 'block';
                        btn.disabled = false;
                        btn.textContent = 'Redefinir senha';
                    }
                }
 
                // Submit com Enter
                document.addEventListener('keydown', function(e) {
                    if (e.key === 'Enter') submitReset();
                });
            </script>
        </body>
        </html>
    """.trimIndent()

    private fun buildResetSuccessHtml(): String = """
        <!DOCTYPE html>
        <html>
        <head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0">
        <title>Senha Redefinida — Loobby</title></head>
        <body style="font-family: -apple-system, sans-serif; background: #0f1117; color: #e0e0e0; display: flex; justify-content: center; align-items: center; min-height: 100vh; margin: 0;">
            <div style="text-align: center; max-width: 400px; padding: 40px;">
                <div style="font-size: 64px; margin-bottom: 16px;">✅</div>
                <h1 style="color: #4ade80; margin-bottom: 12px;">Senha redefinida!</h1>
                <p style="color: #a0a0a0; line-height: 1.6;">Sua senha foi alterada com sucesso. Você já pode voltar ao app e fazer login com a nova senha.</p>
            </div>
        </body>
        </html>
    """.trimIndent()

    private fun buildResetErrorHtml(message: String): String = """
        <!DOCTYPE html>
        <html>
        <head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0">
        <title>Erro — Loobby</title></head>
        <body style="font-family: -apple-system, sans-serif; background: #0f1117; color: #e0e0e0; display: flex; justify-content: center; align-items: center; min-height: 100vh; margin: 0;">
            <div style="text-align: center; max-width: 400px; padding: 40px;">
                <div style="font-size: 64px; margin-bottom: 16px;">❌</div>
                <h1 style="color: #ef4444; margin-bottom: 12px;">Erro</h1>
                <p style="color: #a0a0a0; line-height: 1.6;">$message</p>
                <p style="color: #707070; font-size: 13px; margin-top: 24px;">Abra o app e solicite uma nova redefinição de senha.</p>
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