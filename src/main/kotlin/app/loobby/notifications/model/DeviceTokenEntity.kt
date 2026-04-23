package app.loobby.notifications.model

import jakarta.persistence.Column
import jakarta.persistence.Entity
import jakarta.persistence.EnumType
import jakarta.persistence.Enumerated
import jakarta.persistence.Id
import jakarta.persistence.Table
import org.hibernate.annotations.CreationTimestamp
import org.hibernate.annotations.UpdateTimestamp
import java.time.Instant
import java.util.UUID

/**
 * Token FCM de um device. A PK é o próprio token (único) — se o usuário
 * trocar de conta no mesmo device, fazemos upsert por token atualizando user_id.
 */
@Entity
@Table(name = "device_tokens")
open class DeviceTokenEntity(

    @Id
    @Column(name = "token", nullable = false)
    var token: String,

    @Column(name = "user_id", nullable = false)
    var userId: UUID,

    @Enumerated(EnumType.STRING)
    @Column(name = "platform", nullable = false)
    var platform: DevicePlatform,

    @CreationTimestamp
    @Column(name = "created_at", nullable = false, updatable = false)
    var createdAt: Instant? = null,

    @UpdateTimestamp
    @Column(name = "updated_at", nullable = false)
    var updatedAt: Instant? = null

) {
    protected constructor() : this(
        token = "",
        userId = UUID.randomUUID(),
        platform = DevicePlatform.ANDROID
    )
}
