package app.loobby.events.dto

import jakarta.validation.constraints.NotBlank
import jakarta.validation.constraints.Size

/**
 * Body do POST /public/c/{token}/rsvps/start.
 *
 * Primeira etapa do fluxo de confirmação via link público:
 * o usuário informa nome e telefone; o backend cria um registro em
 * event_rsvp_pending e devolve o telefone normalizado em E.164 para que
 * o frontend dispare o OTP via Firebase Phone Auth.
 */
data class StartConfirmationRequest(

    @field:NotBlank
    @field:Size(max = 100)
    val name: String,

    @field:NotBlank
    @field:Size(max = 30)
    val phone: String
)
