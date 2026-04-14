package app.loobby.users.repo

import app.loobby.users.model.UserGoogleCredentialsEntity
import org.springframework.data.jpa.repository.JpaRepository
import java.util.UUID

interface UserGoogleCredentialsRepository : JpaRepository<UserGoogleCredentialsEntity, UUID> {
    fun findByGoogleId(googleId: String): UserGoogleCredentialsEntity?
}