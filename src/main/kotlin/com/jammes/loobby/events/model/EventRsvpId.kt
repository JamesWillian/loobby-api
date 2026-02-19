package com.jammes.loobby.events.model

import java.io.Serializable
import java.util.UUID

data class EventRsvpId(
    var eventId: UUID? = null,
    var userId: UUID? = null
) : Serializable
