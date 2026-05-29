package app.loobby.games.dto

data class GameSearchResponse(
    val page: Int,
    val count: Int,
    val results: List<GameSummaryResponse>,
)
