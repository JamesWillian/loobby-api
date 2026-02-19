package com.jammes.loobby.events.dto

import java.math.BigDecimal

data class SportEventDetailsResponse(
    val durationMinutes: Int,
    val arena: String?,
    val pricePerPlayer: BigDecimal,
    val maxPlayers: Int?,
    val acceptReserve: Boolean
)
