package com.jammes.loobby.users.model

import jakarta.persistence.*
import org.hibernate.annotations.CreationTimestamp
import java.time.Instant
import java.util.UUID

@Entity
@Table(name = "users")
open class UserEntity(

    @Id
    @Column(name = "id", nullable = false)
    var id: UUID,

    @Column(name = "username", nullable = false, length = 50)
    var username: String,

    @Column(name = "displayname", nullable = true, length = 80)
    var displayname: String? = null,

    @Column(name = "avatar_url", nullable = true, length = 500)
    var avatarUrl: String? = null,

    @Column(name="created_at", insertable=false, updatable=false)
    var createdAt: Instant? = null

) {
    // JPA precisa de construtor vazio
    protected constructor() : this(
        id = UUID.randomUUID(),
        username = "",
        displayname = null,
        avatarUrl = null,
        createdAt = null
    )
}
