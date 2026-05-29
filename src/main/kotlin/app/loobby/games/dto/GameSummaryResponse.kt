package app.loobby.games.dto

import java.math.BigDecimal

/** Versão enxuta usada em listas/resultados de busca. */
data class GameSummaryResponse(
    val id: String,
    val slug: String?,
    val name: String,
    val backgroundImage: String?,
    val released: String?,
    val rating: BigDecimal?,
    val metacritic: Int?,
)
