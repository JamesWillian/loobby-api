package app.loobby.events.teams.dto

import java.util.UUID

data class UpdateTeamPlayerRequest(
    val newTeamId: UUID? = null,
    val role: String? = null
)
