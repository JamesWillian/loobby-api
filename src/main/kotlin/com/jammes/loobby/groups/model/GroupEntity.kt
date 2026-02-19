package com.jammes.loobby.groups.model

import jakarta.persistence.*
import java.time.Instant
import java.util.UUID

@Entity
@Table(name = "groups")
open class GroupEntity(

    @Id
    @Column(name = "id", nullable = false)
    var id: UUID = UUID.randomUUID(),

    @Column(name = "name", nullable = false)
    var name: String,

    @Column(name = "invite_code", nullable = false, unique = true)
    var inviteCode: String,

    @Column(name = "image_url")
    var imageUrl: String? = null,

    @Column(name = "owner_id", nullable = false)
    var ownerId: UUID,

    @Column(name = "created_at", insertable=false, updatable = false)
    var createdAt: Instant? = null,

    ) {
    protected constructor() : this(
        id = UUID.randomUUID(),
        name = "",
        inviteCode = "",
        imageUrl = null,
        ownerId = UUID.randomUUID(),
        createdAt = null
    )
}
