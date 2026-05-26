package app.loobby.events.service

import app.loobby.events.model.EventEntity
import app.loobby.events.model.EventLinkTokenEntity
import app.loobby.events.repo.EventLinkTokenRepository
import app.loobby.events.repo.EventRepository
import app.loobby.groups.repo.GroupRepository
import org.springframework.beans.factory.annotation.Value
import org.springframework.security.access.AccessDeniedException
import org.springframework.stereotype.Service
import org.springframework.transaction.annotation.Transactional
import java.security.MessageDigest
import java.security.SecureRandom
import java.time.Duration
import java.time.Instant
import java.util.Base64
import java.util.UUID

/**
 * Resultado da criação de um link público — carrega o token cru, que NÃO é
 * persistido. Existe apenas para o controller devolver na resposta.
 */
data class CreatedLinkToken(
    val entity: EventLinkTokenEntity,
    val rawToken: String,
    val url: String
)

/**
 * Geração, resolução e revogação de tokens públicos de evento.
 *
 * Modelo de segurança:
 *  - Token cru = 32 bytes aleatórios codificados em base64url (~43 chars).
 *  - Banco guarda apenas SHA-256(token_cru). Se a base vazar, ninguém consegue
 *    usar os tokens — precisa do valor cru, que só foi exibido no momento da
 *    criação.
 *  - Validade controlada por [EventLinkTokenEntity.expiresAt] e [revokedAt].
 */
@Service
class LinkTokenService(
    private val tokenRepository: EventLinkTokenRepository,
    private val eventRepository: EventRepository,
    private val groupRepository: GroupRepository,

    @Value("\${loobby.public.web-base-url}")
    private val webBaseUrl: String,

    /** Quantas horas após a data do evento o link continua válido. */
    @Value("\${loobby.public.link-token-extra-hours:24}")
    private val linkTokenExtraHours: Long
) {

    private val secureRandom = SecureRandom()

    /**
     * Gera um novo token público para o evento. Apenas o criador do evento OU
     * o dono do grupo ao qual o evento pertence (quando houver) podem chamar.
     * Retorna o token cru — o chamador é responsável por devolvê-lo ao cliente,
     * pois ele NÃO fica armazenado.
     */
    @Transactional
    fun createForEvent(eventId: UUID, requesterUserId: UUID): CreatedLinkToken {
        val event = eventRepository.findById(eventId)
            .orElseThrow { IllegalArgumentException("Event not found") }

        assertCanManageEventLinks(event, requesterUserId)

        val rawToken = generateRawToken()
        val expiresAt = event.scheduledDatetime.plus(Duration.ofHours(linkTokenExtraHours))

        val saved = tokenRepository.save(
            EventLinkTokenEntity(
                eventId = event.id,
                tokenHash = sha256Hex(rawToken),
                createdByUserId = requesterUserId,
                expiresAt = expiresAt
            )
        )

        return CreatedLinkToken(
            entity = saved,
            rawToken = rawToken,
            url = "${webBaseUrl.trimEnd('/')}/c/$rawToken"
        )
    }

    /**
     * Resolve um token cru recebido na URL pública. Retorna a entidade
     * correspondente, ou null se o token não existir, estiver expirado ou
     * tiver sido revogado.
     */
    @Transactional(readOnly = true)
    fun resolve(rawToken: String): EventLinkTokenEntity? {
        if (rawToken.isBlank()) return null
        val entity = tokenRepository.findByTokenHash(sha256Hex(rawToken)) ?: return null
        return if (entity.isUsable()) entity else null
    }

    /**
     * Lista todos os tokens (ativos, expirados e revogados) de um evento.
     * Mesma regra de autorização da geração: criador do evento OU dono do grupo.
     */
    @Transactional(readOnly = true)
    fun listForEvent(eventId: UUID, requesterUserId: UUID): List<EventLinkTokenEntity> {
        val event = eventRepository.findById(eventId)
            .orElseThrow { IllegalArgumentException("Event not found") }
        assertCanManageEventLinks(event, requesterUserId)
        return tokenRepository.findByEventId(eventId)
    }

    /**
     * Revoga um token: marca [EventLinkTokenEntity.revokedAt] = agora.
     * Idempotente — se já estava revogado, mantém o timestamp original.
     * Mesma regra de autorização da geração: criador do evento OU dono do grupo.
     */
    @Transactional
    fun revoke(eventId: UUID, tokenId: UUID, requesterUserId: UUID) {
        val token = tokenRepository.findById(tokenId)
            .orElseThrow { IllegalArgumentException("Token not found") }

        if (token.eventId != eventId) {
            throw IllegalArgumentException("Token does not belong to this event")
        }

        val event = eventRepository.findById(eventId)
            .orElseThrow { IllegalArgumentException("Event not found") }

        assertCanManageEventLinks(event, requesterUserId)

        if (token.revokedAt == null) {
            token.revokedAt = Instant.now()
            tokenRepository.save(token)
        }
    }

    /**
     * Permite gerenciar links do evento se o requester for:
     *  - o criador do evento (event.ownerId), ou
     *  - o dono do grupo ao qual o evento pertence (event.groupId → group.ownerId).
     *
     * Para eventos instantâneos (sem groupId), só o criador é autorizado.
     */
    private fun assertCanManageEventLinks(event: EventEntity, requesterUserId: UUID) {
        if (event.ownerId == requesterUserId) return

        val groupId = event.groupId
        if (groupId != null) {
            val group = groupRepository.findById(groupId).orElse(null)
            if (group != null && group.ownerId == requesterUserId) return
        }

        throw AccessDeniedException("Only the event owner or the group owner can manage share links")
    }

    // -------------------------------------------------------------------------
    // Internals
    // -------------------------------------------------------------------------

    private fun generateRawToken(): String {
        val bytes = ByteArray(32)
        secureRandom.nextBytes(bytes)
        return Base64.getUrlEncoder().withoutPadding().encodeToString(bytes)
    }

    private fun sha256Hex(input: String): String {
        val digest = MessageDigest.getInstance("SHA-256")
            .digest(input.toByteArray(Charsets.UTF_8))
        val sb = StringBuilder(digest.size * 2)
        for (b in digest) {
            sb.append(HEX[(b.toInt() ushr 4) and 0x0F])
            sb.append(HEX[b.toInt() and 0x0F])
        }
        return sb.toString()
    }

    companion object {
        private val HEX = "0123456789abcdef".toCharArray()
    }
}
