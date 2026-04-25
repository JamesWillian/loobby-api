package app.loobby.events.teams.dto

data class AutoGenerateTeamsRequest(
    val teamSize: Int? = null,
    val teamCount: Int? = null,
    /** Se true, jogadores com RSVP RESERVE viram um time "Reserva". */
    val includeReserves: Boolean = true
)
