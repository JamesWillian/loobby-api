package com.jammes.loobby.users.dto

import java.time.Instant
import java.util.UUID

data class UserProfileResponse(
    val id: UUID,
    val username: String,
    val displayname: String?,
    val avatarUrl: String?,
    val createdAt: Instant?
)