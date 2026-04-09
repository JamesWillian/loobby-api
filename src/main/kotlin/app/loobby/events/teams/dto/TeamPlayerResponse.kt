package app.loobby.events.teams.dto

import java.util.UUID

data class TeamPlayerResponse(
    val userId: UUID,
    val role: String?,
    val username: String,
    val displayname: String?,
    val avatarUrl: String?
)
