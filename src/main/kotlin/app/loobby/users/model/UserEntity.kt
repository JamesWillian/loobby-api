package app.loobby.users.model

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

    // 0 = ANON, 1 = EMAIL, 2 = GOOGLE
    @Column(name = "auth_provider", nullable = false)
    var authProvider: Int = 0,

    /**
     * Telefone no formato E.164 (ex: "+5511999998888"). Único quando presente.
     * Preenchido para usuários criados a partir de confirmações via link público
     * (após OTP no Firebase Phone Auth) e para usuários que cadastrarem o número
     * no signup do app.
     */
    @Column(name = "phone_e164", nullable = true, length = 20, unique = true)
    var phoneE164: String? = null,

    @Column(name="created_at", insertable=false, updatable=false)
    var createdAt: Instant? = null

) {
    // JPA precisa de construtor vazio
    protected constructor() : this(
        id = UUID.randomUUID(),
        username = "",
        displayname = null,
        avatarUrl = null,
        authProvider = 0,
        phoneE164 = null,
        createdAt = null
    )
}
