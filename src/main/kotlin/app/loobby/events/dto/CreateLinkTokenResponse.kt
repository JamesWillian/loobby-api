package app.loobby.events.dto

import java.time.Instant
import java.util.UUID

/**
 * Resposta para a geração de um novo link público de RSVP.
 *
 * IMPORTANTE: o campo [token] é o valor cru, que NÃO fica armazenado no
 * banco (guardamos só o hash SHA-256). É devolvido uma única vez, no
 * momento da criação. O cliente deve persistir/compartilhar a [url] ou
 * o [token] imediatamente, pois não há como recuperá-los depois.
 */
data class CreateLinkTokenResponse(
    val id: UUID,
    val eventId: UUID,
    val token: String,
    val url: String,
    val expiresAt: Instant
)
