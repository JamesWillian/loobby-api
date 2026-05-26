package app.loobby.events.controller

import app.loobby.common.phone.PhoneNormalizer
import app.loobby.events.dto.ConfirmConfirmationRequest
import app.loobby.events.dto.ConfirmConfirmationResponse
import app.loobby.events.dto.PublicEventResponse
import app.loobby.events.dto.StartConfirmationRequest
import app.loobby.events.dto.StartConfirmationResponse
import app.loobby.events.service.PublicRsvpRateLimiter
import app.loobby.events.service.PublicRsvpService
import jakarta.servlet.http.HttpServletRequest
import jakarta.validation.Valid
import org.springframework.http.HttpStatus
import org.springframework.web.bind.annotation.*
import org.springframework.web.server.ResponseStatusException

/**
 * Endpoints PÚBLICOS (sem JWT) consumidos pela página web compartilhada via
 * WhatsApp: https://loobby.app/c/{token}
 *
 * Fluxo individual em duas etapas, com OTP por Firebase Phone Auth:
 *   - POST /rsvps/start    → registra intenção, devolve phone para o cliente
 *                            disparar Firebase Phone Auth
 *   - POST /rsvps/confirm  → valida ID token do Firebase e promove para event_rsvps
 *
 * O `permitAll` em /public/... está configurado no SecurityConfig.
 */
@RestController
@RequestMapping("/public/c/{token}")
class PublicRsvpController(
    private val publicRsvpService: PublicRsvpService,
    private val rateLimiter: PublicRsvpRateLimiter
) {

    /** Detalhes públicos do evento — usado pra renderizar a tela inicial do link. */
    @GetMapping
    fun getEvent(@PathVariable token: String): PublicEventResponse =
        publicRsvpService.getPublicEvent(token)

    /**
     * Etapa 1: cliente submete nome + telefone. Backend cria pending row e
     * devolve o telefone normalizado em E.164.
     *
     * Rate limits:
     *  - 10 chamadas/min por IP
     *  - 30 chamadas/hora por link
     *  - 1 chamada/60s por telefone (anti-spam de SMS no Firebase)
     */
    @PostMapping("/rsvps/start")
    fun start(
        @PathVariable token: String,
        @Valid @RequestBody req: StartConfirmationRequest,
        request: HttpServletRequest
    ): StartConfirmationResponse {
        val phoneE164 = try {
            PhoneNormalizer.toE164(req.phone)
        } catch (e: IllegalArgumentException) {
            throw ResponseStatusException(HttpStatus.BAD_REQUEST, e.message)
        }

        val ip = resolveClientIp(request)
        rateLimiter.checkRequest(ip, token)
        rateLimiter.checkPhoneCooldown(phoneE164)

        return publicRsvpService.startConfirmation(token, req.name, phoneE164)
    }

    /**
     * Etapa 2: cliente envia o ID token do Firebase + o pendingId devolvido na
     * etapa 1. Backend valida, promove para event_rsvps e apaga o pending.
     *
     * Rate limits: mesmos do /start (sem cooldown por telefone — Firebase já
     * limita por conta dele).
     */
    @PostMapping("/rsvps/confirm")
    fun confirm(
        @PathVariable token: String,
        @Valid @RequestBody req: ConfirmConfirmationRequest,
        request: HttpServletRequest
    ): ConfirmConfirmationResponse {
        val ip = resolveClientIp(request)
        rateLimiter.checkRequest(ip, token)

        return publicRsvpService.confirmConfirmation(
            rawToken = token,
            pendingId = req.pendingId,
            firebaseIdToken = req.firebaseIdToken
        )
    }

    /**
     * Resolve o IP do cliente respeitando X-Forwarded-For. Assume que o
     * backend está atrás de um proxy/load balancer confiável — se for
     * exposto diretamente, configurar `server.forward-headers-strategy`
     * no application.yaml para evitar spoof do header.
     */
    private fun resolveClientIp(request: HttpServletRequest): String {
        val forwarded = request.getHeader("X-Forwarded-For")
            ?.split(",")
            ?.firstOrNull()
            ?.trim()
        return if (!forwarded.isNullOrBlank()) forwarded else request.remoteAddr
    }
}
