package com.jammes.loobby.events.model

import jakarta.persistence.Column
import jakarta.persistence.Entity
import jakarta.persistence.Id
import jakarta.persistence.Table
import java.math.BigDecimal
import java.util.UUID

@Entity
@Table(name = "sport_events")
open class SportEventEntity(

    @Id
    @Column(name = "event_id", nullable = false)
    var eventId: UUID,

    @Column(name = "duration_minutes", nullable = false)
    var durationMinutes: Int,

    @Column(name = "arena")
    var arena: String? = null,

    @Column(name = "price_per_player", nullable = false)
    var pricePerPlayer: BigDecimal = BigDecimal.ZERO,

    @Column(name = "max_players")
    var maxPlayers: Int? = null,

    @Column(name = "accept_reserve", nullable = false)
    var acceptReserve: Boolean = false

) {
    protected constructor() : this(
        eventId = UUID.randomUUID(),
        durationMinutes = 60,
        arena = null,
        pricePerPlayer = BigDecimal.ZERO,
        maxPlayers = null,
        acceptReserve = false
    )
}
