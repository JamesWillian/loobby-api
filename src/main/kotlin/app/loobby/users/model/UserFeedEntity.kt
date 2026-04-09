package app.loobby.users.model

import jakarta.persistence.Column
import org.hibernate.annotations.Immutable
import jakarta.persistence.Entity
import jakarta.persistence.Id
import jakarta.persistence.Table
import java.util.UUID
import kotlin.time.Instant

@Entity
@Table(name = "user_feed")
@Immutable
data class UserFeedEntity (
    @Id
    val id: UUID,

    @Column(name = "user_id")
    val userId: UUID,

    val name: String,

    @Column(name = "image_url")
    val imageUrl: String?,

    @Column(name = "entry_type")
    val entryType: String,

    @Column(name = "is_finished")
    val isFinished: Boolean
)