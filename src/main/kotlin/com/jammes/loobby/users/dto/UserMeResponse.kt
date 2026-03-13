package com.jammes.loobby.users.dto

import com.fasterxml.jackson.annotation.JsonProperty
import java.time.Instant
import java.util.UUID

data class UserMeResponse(
    val id: UUID,
    val username: String,
    val displayname: String?,
    val avatarUrl: String?,
    @get:JsonProperty("isAnonymous")
    val isAnonymous: Boolean,
    val roles: List<String>,
    val email: String?,            // null para anônimo
    val createdAt: Instant?
)
