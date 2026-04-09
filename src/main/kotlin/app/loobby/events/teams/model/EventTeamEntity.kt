package app.loobby.events.teams.model

import jakarta.persistence.*
import java.util.UUID

@Entity
@Table(
    name = "event_teams",
    uniqueConstraints = [
        UniqueConstraint(
            name = "event_teams_event_order_unique",
            columnNames = ["event_id", "\"order\""]
        ),
        UniqueConstraint(
            name = "event_teams_event_name_unique",
            columnNames = ["event_id", "name"]
        )
    ]
)
open class EventTeamEntity(

    @Id
    @Column(name = "id", nullable = false)
    var id: UUID = UUID.randomUUID(),

    @Column(name = "event_id", nullable = false)
    var eventId: UUID,

    // "order" é palavra reservada, então mapeio com outro nome de propriedade
    @Column(name = "\"order\"", nullable = false)
    var orderIndex: Int = 0,

    @Column(name = "name", nullable = false)
    var name: String,

    @Column(name = "color")
    var color: String? = null

) {
    protected constructor() : this(
        id = UUID.randomUUID(),
        eventId = UUID.randomUUID(),
        orderIndex = 0,
        name = "",
        color = null
    )
}
