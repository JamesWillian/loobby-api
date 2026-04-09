package app.loobby.events.dto

import jakarta.validation.constraints.NotBlank

data class GameplayEventDetailsRequest(
    val gameId: String? = null,

    @field:NotBlank
    val gameName: String
)
