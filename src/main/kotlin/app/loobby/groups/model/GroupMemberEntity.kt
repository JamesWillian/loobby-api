package app.loobby.groups.model

import jakarta.persistence.*
import java.util.UUID

@Entity
@Table(
    name = "group_members",
    uniqueConstraints = [
        UniqueConstraint(
            name = "group_members_unique",
            columnNames = ["group_id", "user_id"]
        )
    ]
)
open class GroupMemberEntity(

    @Id
    @Column(name = "id", nullable = false)
    var id: UUID = UUID.randomUUID(),

    @Column(name = "group_id", nullable = false)
    var groupId: UUID,

    @Column(name = "user_id", nullable = false)
    var userId: UUID

) {
    protected constructor() : this(
        id = UUID.randomUUID(),
        groupId = UUID.randomUUID(),
        userId = UUID.randomUUID()
    )
}
