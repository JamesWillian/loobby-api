package com.jammes.loobby.auth.controller

import com.jammes.loobby.auth.dto.AuthResponse
import com.jammes.loobby.auth.dto.LoginRequest
import com.jammes.loobby.auth.dto.RefreshTokenRequest
import com.jammes.loobby.auth.dto.RegisterRequest
import com.jammes.loobby.auth.service.AuthService
import jakarta.validation.Valid
import org.springframework.security.core.annotation.AuthenticationPrincipal
import org.springframework.security.oauth2.jwt.Jwt
import org.springframework.security.oauth2.jwt.JwtDecoder
import org.springframework.security.oauth2.jwt.JwtException
import org.springframework.web.bind.annotation.*
import java.util.UUID

@RestController
@RequestMapping("/auth")
class AuthController(
    private val authService: AuthService,
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
        return authService.register(userId, req)
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