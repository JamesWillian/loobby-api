package app.loobby.events.controller

import app.loobby.events.dto.CreateLinkTokenResponse
import app.loobby.events.dto.LinkTokenSummary
import app.loobby.events.service.LinkTokenService
import org.springframework.http.HttpStatus
import org.springframework.security.core.annotation.AuthenticationPrincipal
import org.springframework.security.oauth2.jwt.Jwt
import org.springframework.web.bind.annotation.*
import java.util.UUID

/**
 * Endpoints autenticados que o host usa para gerenciar links públicos de
 * confirmação de presença em seus eventos.
 *
 * Por convenção do projeto, este controller fica no namespace /events/... —
 * que já exige JWT pelo SecurityConfig — sem prefixo /host.
 */
@RestController
@RequestMapping("/events/{eventId}/link-tokens")
class HostLinkTokenController(
    private val linkTokenService: LinkTokenService
) {

    /**
     * Gera um novo link público para o evento. Só o owner do evento pode chamar
     * (verificação feita no service). O `token` cru é devolvido UMA vez aqui —
     * o cliente deve persistir a `url` imediatamente.
     */
    @PostMapping
    @ResponseStatus(HttpStatus.CREATED)
    fun create(
        @AuthenticationPrincipal jwt: Jwt,
        @PathVariable eventId: UUID
    ): CreateLinkTokenResponse {
        val userId = UUID.fromString(jwt.subject)
        val created = linkTokenService.createForEvent(eventId, userId)
        return CreateLinkTokenResponse(
            id = created.entity.id,
            eventId = created.entity.eventId,
            token = created.rawToken,
            url = created.url,
            expiresAt = created.entity.expiresAt
        )
    }

    /**
     * Lista todos os links já gerados pro evento (ativos, expirados, revogados).
     * Não devolve o token cru — o valor original só existiu no [create].
     */
    @GetMapping
    fun list(
        @AuthenticationPrincipal jwt: Jwt,
        @PathVariable eventId: UUID
    ): List<LinkTokenSummary> {
        val userId = UUID.fromString(jwt.subject)
        return linkTokenService.listForEvent(eventId, userId)
            .map { LinkTokenSummary.from(it) }
    }

    /**
     * Revoga um link, tornando-o inválido a partir de agora. Operação idempotente:
     * 204 mesmo se já estava revogado.
     */
    @DeleteMapping("/{tokenId}")
    @ResponseStatus(HttpStatus.NO_CONTENT)
    fun revoke(
        @AuthenticationPrincipal jwt: Jwt,
        @PathVariable eventId: UUID,
        @PathVariable tokenId: UUID
    ) {
        val userId = UUID.fromString(jwt.subject)
        linkTokenService.revoke(eventId, tokenId, userId)
    }
}
