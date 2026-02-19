package com.jammes.loobby.events.teams.dto

import jakarta.validation.constraints.NotBlank

data class CreateTeamRequest(

    @field:NotBlank
    val name: String,

    val color: String? = null,

    // se null, o backend define (último + 1)
    val order: Int? = null,

    val players: List<TeamPlayerRequest> = emptyList()
)
