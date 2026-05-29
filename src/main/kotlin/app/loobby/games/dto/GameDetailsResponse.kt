package app.loobby.games.dto

import com.fasterxml.jackson.databind.JsonNode
import java.math.BigDecimal

/** Detalhes completos usados na tela de detalhe do jogo/evento. */
data class GameDetailsResponse(
    val id: String,
    val slug: String?,
    val name: String,
    val backgroundImage: String?,
    val released: String?,
    val rating: BigDecimal?,
    val metacritic: Int?,
    val descriptionRaw: String?,
    val genres: JsonNode?,
    val platforms: JsonNode?,
)
