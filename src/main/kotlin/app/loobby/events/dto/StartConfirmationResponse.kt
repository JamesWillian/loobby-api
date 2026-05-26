package app.loobby.events.dto

import java.util.UUID

/**
 * Resposta do POST /public/c/{token}/rsvps/start.
 *
 * Carrega o telefone já normalizado em E.164 — esse é o valor que o frontend
 * deve passar para o Firebase JS SDK no signInWithPhoneNumber().
 *
 * Quando [status] = NEED_OTP, [pendingId] aponta para o registro em
 * event_rsvp_pending que será promovido para event_rsvps após a confirmação
 * do OTP.
 *
 * Quando [status] = ALREADY_CONFIRMED, o telefone já tem RSVP confirmada para
 * o evento — o frontend pode pular o OTP e exibir o resultado direto. Nesse
 * caso [pendingId] é nulo.
 */
data class StartConfirmationResponse(
    val status: StartConfirmationStatus,
    val phone: String,
    val pendingId: UUID? = null
)

enum class StartConfirmationStatus {
    /** Cliente deve disparar o Firebase Phone Auth e chamar /rsvps/confirm. */
    NEED_OTP,

    /** Já existe RSVP confirmada para esse telefone neste evento. */
    ALREADY_CONFIRMED
}
