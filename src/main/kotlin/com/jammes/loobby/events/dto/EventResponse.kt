package com.jammes.loobby.events.dto

import com.fasterxml.jackson.annotation.JsonProperty
import com.jammes.loobby.events.model.EventType
import java.time.Instant
import java.util.UUID

data class EventResponse(
    val id: UUID,
    val eventType: EventType,
    val groupId: UUID?,
    @get:JsonProperty("isInstant")
    val isInstant: Boolean,
    val ownerId: UUID,
    val scheduledDatetime: Instant,
    val name: String,
    val description: String?,
    val inviteCode: String,
    val createdAt: Instant?
)
