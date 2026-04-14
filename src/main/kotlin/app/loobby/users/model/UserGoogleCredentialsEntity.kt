package app.loobby.users.model

import jakarta.persistence.*
import org.hibernate.annotations.CreationTimestamp
import java.time.Instant
import java.util.UUID

@Entity
@Table(
    name = "user_google_credentials",
    indexes = [
        Index(name = "ix_google_creds_google_id", columnList = "google_id", unique = true)
    ]
)
class UserGoogleCredentialsEntity(

    @Id
    @Column(name = "user_id", nullable = false, unique = true)
    val userId: UUID,

    @Column(name = "google_id", nullable = false, unique = true)
    val googleId: String,

    @Column(name = "email", nullable = false)
    val email: String,

    @CreationTimestamp
    @Column(name = "created_at", nullable = false, updatable = false)
    var createdAt: Instant? = null

) {
    protected constructor() : this(
        userId = UUID.randomUUID(),
        googleId = "",
        email = ""
    )
}