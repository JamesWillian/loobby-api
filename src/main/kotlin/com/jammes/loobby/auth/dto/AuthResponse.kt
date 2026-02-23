package com.jammes.loobby.auth.dto

import java.util.UUID

data class AuthResponse(
    val accessToken: String,
    val refreshToken: String,
    val expiresIn: Long,          // em segundos (do access token)
    val userId: UUID,
    val username: String,
    val roles: List<String>
)