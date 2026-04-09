package app.loobby.events.teams.model

import java.io.Serializable
import java.util.UUID

data class TeamPlayerId(
    var teamId: UUID? = null,
    var userId: UUID? = null
) : Serializable
