package app.loobby.events.teams.dto

data class UpdateTeamRequest(
    val name: String? = null,
    val color: String? = null,
    val order: Int? = null
)
