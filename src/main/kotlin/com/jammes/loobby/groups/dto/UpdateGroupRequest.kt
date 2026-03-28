package com.jammes.loobby.groups.dto

import jakarta.validation.constraints.NotBlank
import jakarta.validation.constraints.Size

data class UpdateGroupRequest(

    @field:NotBlank
    @field:Size(min = 1, max = 100)
    val name: String
)