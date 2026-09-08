package app.loobby.common.errors

import jakarta.servlet.http.HttpServletRequest
import org.springframework.http.HttpStatus
import org.springframework.http.ResponseEntity
import org.springframework.security.access.AccessDeniedException
import org.springframework.security.oauth2.jwt.JwtException
import org.springframework.validation.FieldError
import org.springframework.web.bind.MethodArgumentNotValidException
import org.springframework.web.bind.annotation.ExceptionHandler
import org.springframework.web.bind.annotation.RestControllerAdvice
import org.springframework.web.server.ResponseStatusException
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

    @ExceptionHandler(JwtException::class)
    fun handleJwtException(
        ex: JwtException,
        request: HttpServletRequest
    ): ResponseEntity<ApiErrorResponse> {
        val status = HttpStatus.UNAUTHORIZED
        val body = ApiErrorResponse(
            timestamp = Instant.now(),
            status = status.value(),
            error = status.reasonPhrase,
            message = "Refresh token expired or invalid",
            path = request.requestURI
        )
        return ResponseEntity.status(status).body(body)
    }

    // -----------------------------
    // AccessDeniedException - falha de autorizacao
    //
    // Mesmo motivo do handler abaixo: sem um handler dedicado, o catch-all
    // engolia a excecao e o cliente recebia 500 "Unexpected error" no lugar de
    // 403. A autorizacao em si sempre funcionou -- so o status e que mentia.
    // Lancada por GroupService, GroupMemberService e EventService.
    // -----------------------------
    @ExceptionHandler(AccessDeniedException::class)
    fun handleAccessDenied(
        ex: AccessDeniedException,
        request: HttpServletRequest
    ): ResponseEntity<ApiErrorResponse> {
        val status = HttpStatus.FORBIDDEN

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
    // ResponseStatusException - o status ja vem declarado na excecao
    //
    // Precisa de handler proprio: o @ExceptionHandler(Exception::class) abaixo
    // tambem casa com ela (e uma Exception), e o ExceptionHandlerExceptionResolver
    // roda antes do ResponseStatusExceptionResolver do Spring. Sem isto, todo
    // ResponseStatusException do projeto virava 500 "Unexpected error" -- inclusive
    // o 404 de /games/{id} e os 401 de senha incorreta em UsersService.
    // -----------------------------
    @ExceptionHandler(ResponseStatusException::class)
    fun handleResponseStatusException(
        ex: ResponseStatusException,
        request: HttpServletRequest
    ): ResponseEntity<ApiErrorResponse> {
        val status = HttpStatus.valueOf(ex.statusCode.value())

        val body = ApiErrorResponse(
            timestamp = Instant.now(),
            status = status.value(),
            error = status.reasonPhrase,
            // reason: so a mensagem. ex.message viria como: 404 NOT_FOUND "Jogo x nao encontrado"
            message = ex.reason,
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
