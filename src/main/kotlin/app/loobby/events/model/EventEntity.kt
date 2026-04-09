package app.loobby.events.model

import jakarta.persistence.*
import org.hibernate.annotations.CreationTimestamp
import org.hibernate.annotations.JdbcTypeCode
import org.hibernate.type.SqlTypes
import java.time.Instant
import java.util.UUID

@Entity
@Table(name = "events")
open class EventEntity(

    @Id
    @Column(name = "id", nullable = false)
    var id: UUID = UUID.randomUUID(),

    @Enumerated(EnumType.STRING)
    @Column(name = "event_type", nullable = false)
    var eventType: EventType,

    @Column(name = "group_id")
    var groupId: UUID? = null,

    @Column(name = "is_instant", nullable = false)
    var isInstant: Boolean = false,

    @Column(name = "owner_id", nullable = false)
    var ownerId: UUID,

    @Column(name = "scheduled_datetime", nullable = false)
    var scheduledDatetime: Instant,

    @Column(name = "name", nullable = false)
    var name: String,

    @Column(name = "description")
    var description: String? = null,

    @Column(name = "invite_code", nullable = false, unique = true)
    var inviteCode: String,

    @Column(name = "created_at", insertable = false, updatable = false)
    var createdAt: Instant? = null

) {
    protected constructor() : this(
        id = UUID.randomUUID(),
        eventType = EventType.GAMEPLAY, // valor default só pro construtor do JPA
        groupId = null,
        isInstant = false,
        ownerId = UUID.randomUUID(),
        scheduledDatetime = Instant.now(),
        name = "",
        description = null,
        inviteCode = ""
    )
}
