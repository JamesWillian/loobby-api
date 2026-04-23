package app.loobby.notifications.dto

import app.loobby.notifications.model.DevicePlatform
import jakarta.validation.constraints.NotBlank

data class RegisterDeviceRequest(

    @field:NotBlank
    val token: String,

    val platform: DevicePlatform
)
