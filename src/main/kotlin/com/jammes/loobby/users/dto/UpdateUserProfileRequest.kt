package com.jammes.loobby.users.dto

import jakarta.validation.constraints.Size

data class UpdateUserProfileRequest(

    @field:Size(min = 3, max = 30, message = "username must be between 3 and 30 characters")
    val username: String? = null,

    @field:Size(max = 50, message = "displayname must be at most 50 characters")
    val displayname: String? = null,

    @field:Size(max = 255, message = "avatarUrl must be at most 255 characters")
    val avatarUrl: String? = null
)