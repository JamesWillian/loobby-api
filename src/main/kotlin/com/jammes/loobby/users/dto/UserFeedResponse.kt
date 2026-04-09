package com.jammes.loobby.users.dto

import com.fasterxml.jackson.annotation.JsonProperty
import java.util.UUID

data class UserFeedResponse(
    val id: UUID,
    val userId: UUID,
    val name: String,
    val imageUrl: String?,
    val entryType: String,
    @get:JsonProperty("isFinished")
    val isFinished: Boolean
)
