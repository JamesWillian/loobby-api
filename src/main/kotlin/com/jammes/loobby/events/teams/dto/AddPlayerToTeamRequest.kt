package com.jammes.loobby.events.teams.dto

import java.util.UUID

data class AddPlayerToTeamRequest(
    val userId: UUID,
    val role: String? = null
)
