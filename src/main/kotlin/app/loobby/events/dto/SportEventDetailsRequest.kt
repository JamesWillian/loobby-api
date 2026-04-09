package app.loobby.events.dto

import java.math.BigDecimal
import jakarta.validation.constraints.NotNull
import jakarta.validation.constraints.Positive

data class SportEventDetailsRequest(

    @field:NotNull
    @field:Positive
    val durationMinutes: Int,

    val arena: String? = null,

    val pricePerPlayer: BigDecimal? = null,

    @field:Positive
    val maxPlayers: Int? = null,

    val acceptReserve: Boolean? = null
)
