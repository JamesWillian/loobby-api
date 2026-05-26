package app.loobby.events.dto

import jakarta.validation.constraints.NotBlank
import jakarta.validation.constraints.NotNull
import java.util.UUID

/**
 * Body do POST /public/c/{token}/rsvps/confirm.
 *
 * Segunda etapa do fluxo: após o Firebase Phone Auth devolver um ID token
 * (prova de posse do telefone), o frontend envia o token + o ID do pending
 * para o backend, que valida, promove para event_rsvps e apaga o pending.
 */
data class ConfirmConfirmationRequest(

    @field:NotNull
    val pendingId: UUID,

    @field:NotBlank
    val firebaseIdToken: String
)
