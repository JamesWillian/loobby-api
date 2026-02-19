package com.jammes.loobby.groups.dto

import java.util.UUID

data class GroupMemberResponse(
    val userId: UUID,
    val username: String,
    val displayname: String?,
    val avatarUrl: String?,
    val isOwner: Boolean
)
