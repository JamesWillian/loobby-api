package app.loobby.users.repo

import app.loobby.users.model.UserCredentialsEntity
import org.springframework.data.jpa.repository.JpaRepository
import java.util.UUID

interface UserCredentialsRepository : JpaRepository<UserCredentialsEntity, UUID> {
    fun findByEmail(email: String): UserCredentialsEntity?
    fun existsByUserId(userId: UUID): Boolean
    fun findByUserId(userId: UUID): UserCredentialsEntity?
    fun findByEmailVerificationToken(token: String): UserCredentialsEntity?
    fun findByPasswordResetToken(token: String): UserCredentialsEntity?
}
