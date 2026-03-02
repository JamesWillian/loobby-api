package com.jammes.loobby.groups.dto

import com.fasterxml.jackson.annotation.JsonProperty
import java.util.UUID

data class GroupMemberResponse(
    val userId: UUID,
    val username: String,
    val displayname: String?,
    val avatarUrl: String?,
    @get:JsonProperty("isOwner")
    val isOwner: Boolean
)
