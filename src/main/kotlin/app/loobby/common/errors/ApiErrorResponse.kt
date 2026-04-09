package app.loobby.common.errors

import java.time.Instant

data class ApiErrorResponse(
    val timestamp: Instant = Instant.now(),
    val status: Int,
    val error: String,
    val message: String?,
    val path: String?,
    val fieldErrors: List<FieldError>? = null
) {
    data class FieldError(
        val field: String,
        val message: String?
    )
}
