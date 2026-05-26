package app.loobby.events.model

import jakarta.persistence.*
import org.hibernate.annotations.CreationTimestamp
import java.time.Instant
import java.util.UUID

@Entity
@Table(name = "event_rsvps")
@IdClass(EventRsvpId::class)
open class EventRsvpEntity(

    @Id
    @Column(name = "event_id", nullable = false)
    var eventId: UUID,

    @Id
    @Column(name = "user_id", nullable = false)
    var userId: UUID,

    @Enumerated(EnumType.STRING)
    @Column(name = "status", nullable = false)
    var status: RsvpStatus,

    @Column(name = "is_paid", nullable = false)
    var isPaid: Boolean = false,

    @Column(name = "obs")
    var obs: String? = null,

    /** Por onde essa confirmação entrou (app autenticado ou link público com OTP). */
    @Enumerated(EnumType.STRING)
    @Column(name = "source", nullable = false, length = 20)
    var source: RsvpSource = RsvpSource.APP,

    /** Nível de confiança da identidade por trás da confirmação. */
    @Enumerated(EnumType.STRING)
    @Column(name = "verification_level", nullable = false, length = 30)
    var verificationLevel: RsvpVerificationLevel = RsvpVerificationLevel.APP_AUTHENTICATED,

    @Column(name = "created_at", insertable = false, updatable = false)
    var createdAt: Instant? = null

) {
    protected constructor() : this(
        eventId = UUID.randomUUID(),
        userId = UUID.randomUUID(),
        status = RsvpStatus.MAYBE,
        isPaid = false,
        obs = null,
        source = RsvpSource.APP,
        verificationLevel = RsvpVerificationLevel.APP_AUTHENTICATED,
        createdAt = null
    )
}
