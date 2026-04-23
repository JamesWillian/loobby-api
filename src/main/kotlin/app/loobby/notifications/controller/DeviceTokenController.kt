package app.loobby.notifications.controller

import app.loobby.notifications.dto.RegisterDeviceRequest
import app.loobby.notifications.service.DeviceTokenService
import jakarta.validation.Valid
import org.springframework.http.HttpStatus
import org.springframework.security.core.annotation.AuthenticationPrincipal
import org.springframework.security.oauth2.jwt.Jwt
import org.springframework.web.bind.annotation.DeleteMapping
import org.springframework.web.bind.annotation.PathVariable
import org.springframework.web.bind.annotation.PostMapping
import org.springframework.web.bind.annotation.RequestBody
import org.springframework.web.bind.annotation.RequestMapping
import org.springframework.web.bind.annotation.ResponseStatus
import org.springframework.web.bind.annotation.RestController
import java.util.UUID

@RestController
@RequestMapping("/devices")
class DeviceTokenController(
    private val service: DeviceTokenService
) {

    /**
     * Registra ou atualiza o FCM token do device para o usuário autenticado.
     * O app chama este endpoint após login e sempre que o token mudar
     * (FirebaseMessagingService.onNewToken no Android).
     */
    @PostMapping("/register")
    @ResponseStatus(HttpStatus.NO_CONTENT)
    fun register(
        @AuthenticationPrincipal jwt: Jwt,
        @Valid @RequestBody request: RegisterDeviceRequest
    ) {
        val userId = UUID.fromString(jwt.subject)
        service.register(userId, request)
    }

    /**
     * Remove o token (chamado no logout). Qualquer usuário autenticado
     * pode remover um token que conhece — não vazamos identificação.
     */
    @DeleteMapping("/{token}")
    @ResponseStatus(HttpStatus.NO_CONTENT)
    fun unregister(
        @AuthenticationPrincipal jwt: Jwt,
        @PathVariable token: String
    ) {
        service.unregister(token)
    }
}
