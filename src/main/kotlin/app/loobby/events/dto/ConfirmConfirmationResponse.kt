package app.loobby.events.dto

import java.util.UUID

/**
 * Resposta do POST /public/c/{token}/rsvps/confirm.
 *
 * Devolvido quando a confirmação foi promovida com sucesso para event_rsvps.
 * Casos de erro (token inválido, mismatch de telefone, pending expirado)
 * são sinalizados via HTTP status code + mensagem padrão de erro do projeto.
 */
data class ConfirmConfirmationResponse(
    val userId: UUID,
    val name: String,
    val phone: String
)
