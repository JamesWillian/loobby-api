package app.loobby.events.model

import jakarta.persistence.Column
import jakarta.persistence.Entity
import jakarta.persistence.Id
import jakarta.persistence.Table
import java.util.UUID

@Entity
@Table(name = "gameplay_events")
open class GameplayEventEntity(

    @Id
    @Column(name = "event_id", nullable = false)
    var eventId: UUID,

    @Column(name = "game_id")
    var gameId: String? = null,

    @Column(name = "game_name", nullable = false)
    var gameName: String,

    ) {
    protected constructor() : this(
        eventId = UUID.randomUUID(),
        gameId = null,
        gameName = ""
    )
}
