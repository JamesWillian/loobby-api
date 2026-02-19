package com.jammes.loobby.events.dto

import com.jammes.loobby.events.model.RsvpStatus
import jakarta.validation.constraints.NotNull

data class UpsertRsvpRequest(

    @field:NotNull
    val status: RsvpStatus,

    // se não vier, mantém o atual (ou false na criação)
    val isPaid: Boolean? = null,

    val obs: String? = null
)
