package app.loobby.events.dto

import app.loobby.events.model.EventType
import java.time.Instant
import java.util.UUID

/**
 * Versão pública (sem dados sensíveis) do evento, retornada para visitantes
 * que possuem o link mas não estão autenticados.
 */
data class PublicEventResponse(
    val id: UUID,
    val eventType: EventType,
    val name: String,
    val description: String?,
    val scheduledDatetime: Instant,
    val confirmedCount: Int,
    val ownerDisplayname: String?
)
