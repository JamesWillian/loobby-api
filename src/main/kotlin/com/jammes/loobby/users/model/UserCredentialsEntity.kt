package com.jammes.loobby.users.model

import jakarta.persistence.*
import org.hibernate.annotations.CreationTimestamp
import java.time.Instant
import java.util.UUID

@Entity
@Table(
    name = "user_credentials",
    indexes = [
        Index(name = "ix_user_credentials_email", columnList = "email", unique = true)
    ]
)
class UserCredentialsEntity(

    @Id
    @Column(name = "user_id", nullable = false)
    var userId: UUID,

    @Column(name = "email", unique = true, nullable = false, length = 254)
    var email: String,

    @Column(name = "password_hash", nullable = false, length = 255)
    var passwordHash: String,

    @Column(name = "roles", nullable = false, length = 200)
    var rolesCsv: String = "USER",

    @CreationTimestamp
    @Column(name = "created_at", nullable = false, updatable = false)
    var createdAt: Instant? = null

) {

    @OneToOne(fetch = FetchType.LAZY)
    @JoinColumn(name = "user_id")
    var user: UserEntity? = null

    // Helpers (não persistidos)
    @get:Transient
    val roles: List<String>
        get() = rolesCsv.split(",", " ")
            .map { it.trim() }
            .filter { it.isNotBlank() }

    fun setRoles(roles: List<String>) {
        rolesCsv = roles.joinToString(",")
    }

    protected constructor() : this(
        userId = UUID.randomUUID(),
        email = "",
        passwordHash = "",
        rolesCsv = "USER",
        createdAt = null
    )
}
