package app.loobby.auth.dto

data class ResetPasswordFormRequest(
    val token: String,
    val password: String
)