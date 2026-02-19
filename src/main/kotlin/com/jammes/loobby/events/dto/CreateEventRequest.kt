package com.jammes.loobby.events.dto

import com.jammes.loobby.events.model.EventType
import jakarta.validation.constraints.FutureOrPresent
import jakarta.validation.constraints.NotBlank
import jakarta.validation.constraints.NotNull
import java.time.Instant

data class CreateEventRequest(

    @field:NotNull
    val eventType: EventType,

    @field:NotBlank
    val name: String,

    val description: String? = null,

    @field:NotNull
    @field:FutureOrPresent
    val scheduledDatetime: Instant,

    // Detalhes específicos (dependem do eventType)
    val gameplay: GameplayEventDetailsRequest? = null,
    val sport: SportEventDetailsRequest? = null
)
