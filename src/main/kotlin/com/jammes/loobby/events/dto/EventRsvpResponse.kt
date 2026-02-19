package com.jammes.loobby.events.dto

import com.fasterxml.jackson.annotation.JsonProperty
import com.jammes.loobby.events.model.RsvpStatus
import java.time.Instant
import java.util.UUID

data class EventRsvpResponse(
    val eventId: UUID,
    val userId: UUID,
    val status: RsvpStatus,
    @get:JsonProperty("isPaid")
    val isPaid: Boolean,
    val obs: String?,
    val createdAt: Instant?,

    val username: String,
    val displayname: String?,
    val avatarUrl: String?,
    val isOwner: Boolean
)
