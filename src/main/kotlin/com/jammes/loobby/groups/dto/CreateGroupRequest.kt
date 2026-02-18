package com.jammes.loobby.groups.dto

import jakarta.validation.constraints.NotBlank

data class CreateGroupRequest(

    @field:NotBlank
    val name: String,

    val imageUrl: String? = null
)
