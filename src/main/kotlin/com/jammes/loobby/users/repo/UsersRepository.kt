package com.jammes.loobby.users.repo

import com.jammes.loobby.users.model.UserEntity
import org.springframework.data.jpa.repository.JpaRepository
import java.util.UUID

interface UsersRepository : JpaRepository<UserEntity, UUID> {
    fun findByUsername(username: String): UserEntity?
}