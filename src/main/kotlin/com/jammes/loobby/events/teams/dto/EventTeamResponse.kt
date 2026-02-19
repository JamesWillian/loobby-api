package com.jammes.loobby.events.teams.dto

import java.util.UUID

data class EventTeamResponse(
    val id: UUID,
    val eventId: UUID,
    val order: Int,
    val name: String,
    val color: String?,
    val players: List<TeamPlayerResponse>
)
