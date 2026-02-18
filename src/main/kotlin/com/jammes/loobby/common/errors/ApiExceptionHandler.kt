package com.jammes.loobby.common.errors

import jakarta.servlet.http.HttpServletRequest
import org.springframework.http.HttpStatus
import org.springframework.http.ResponseEntity
import org.springframework.validation.FieldError
import org.springframework.web.bind.MethodArgumentNotValidException
import org.springframework.web.bind.annotation.ExceptionHandler
import org.springframework.web.bind.annotation.RestControllerAdvice
import java.time.Instant

@RestControllerAdvice
class ApiExceptionHandler {

    // -----------------------------
    // Erros de validação @Valid
    // -----------------------------
    @ExceptionHandler(MethodArgumentNotValidException::class)
    fun handleValidationException(
        ex: MethodArgumentNotValidException,
        request: HttpServletRequest
    ): ResponseEntity<ApiErrorResponse> {
        val status = HttpStatus.BAD_REQUEST

        val fieldErrors = ex.bindingResult.fieldErrors.map {
            ApiErrorResponse.FieldError(
                field = it.field,
                message = resolveFieldErrorMessage(it)
            )
        }

        val body = ApiErrorResponse(
            timestamp = Instant.now(),
            status = status.value(),
            error = status.reasonPhrase,
            message = "Validation failed",
            path = request.requestURI,
            fieldErrors = fieldErrors
        )

        return ResponseEntity.status(status).body(body)
    }

    private fun resolveFieldErrorMessage(error: FieldError): String? {
        // Usa a message resolvida pelo Spring se existir, senão o default
        return error.defaultMessage ?: "Invalid value"
    }

    // -----------------------------
    // IllegalArgument / IllegalState
    // (erros de regra de negócio, tipo "User not found")
    // -----------------------------
    @ExceptionHandler(IllegalArgumentException::class, IllegalStateException::class)
    fun handleIllegalArgumentOrState(
        ex: RuntimeException,
        request: HttpServletRequest
    ): ResponseEntity<ApiErrorResponse> {
        val status = HttpStatus.BAD_REQUEST

        val body = ApiErrorResponse(
            timestamp = Instant.now(),
            status = status.value(),
            error = status.reasonPhrase,
            message = ex.message,
            path = request.requestURI
        )

        return ResponseEntity.status(status).body(body)
    }

    // -----------------------------
    // Fallback - qualquer outra Exception
    // -----------------------------
    @ExceptionHandler(Exception::class)
    fun handleGenericException(
        ex: Exception,
        request: HttpServletRequest
    ): ResponseEntity<ApiErrorResponse> {
        val status = HttpStatus.INTERNAL_SERVER_ERROR

        val body = ApiErrorResponse(
            timestamp = Instant.now(),
            status = status.value(),
            error = status.reasonPhrase,
            message = "Unexpected error",
            path = request.requestURI
        )

        // Aqui você pode logar o erro com mais detalhes (stacktrace)
        // LoggerFactory.getLogger(ApiExceptionHandler::class.java).error("Unexpected error", ex)

        return ResponseEntity.status(status).body(body)
    }
}
