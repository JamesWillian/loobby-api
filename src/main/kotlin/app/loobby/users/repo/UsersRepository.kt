package app.loobby.users.repo

import app.loobby.users.model.UserEntity
import app.loobby.users.model.UserFeedEntity
import org.springframework.data.jpa.repository.JpaRepository
import org.springframework.data.jpa.repository.Query
import org.springframework.data.repository.query.Param
import java.util.UUID

interface UsersRepository : JpaRepository<UserEntity, UUID> {
    fun findByUsername(username: String): UserEntity?
    fun existsByUsernameIgnoreCase(username: String): Boolean

    /** Busca usuário pelo telefone em E.164 — chave do fluxo de confirmação por link/WhatsApp. */
    fun findByPhoneE164(phoneE164: String): UserEntity?
}