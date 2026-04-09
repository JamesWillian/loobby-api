package com.jammes.loobby.users.repo

import com.jammes.loobby.users.model.UserFeedEntity
import org.springframework.data.jpa.repository.JpaRepository
import org.springframework.data.jpa.repository.Query
import org.springframework.data.repository.query.Param
import java.util.UUID

interface UserFeedRepository : JpaRepository<UserFeedEntity, UUID> {

    @Query("select * from user_feed where user_id = :userId", nativeQuery = true)
    fun findByUserId(@Param("userId") userId: UUID): List<UserFeedEntity>
}