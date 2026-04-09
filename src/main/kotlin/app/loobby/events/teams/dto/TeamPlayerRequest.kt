package app.loobby.events.teams.dto

import java.util.UUID

data class TeamPlayerRequest(
    val userId: UUID,
    val role: String? = null
)
