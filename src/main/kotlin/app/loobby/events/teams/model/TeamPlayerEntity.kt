package app.loobby.events.teams.model

import jakarta.persistence.*
import java.util.UUID

@Entity
@Table(name = "team_players")
@IdClass(TeamPlayerId::class)
open class TeamPlayerEntity(

    @Id
    @Column(name = "team_id", nullable = false)
    var teamId: UUID,

    @Id
    @Column(name = "user_id", nullable = false)
    var userId: UUID,

    @Column(name = "role")
    var role: String? = null

) {
    protected constructor() : this(
        teamId = UUID.randomUUID(),
        userId = UUID.randomUUID(),
        role = null
    )
}
