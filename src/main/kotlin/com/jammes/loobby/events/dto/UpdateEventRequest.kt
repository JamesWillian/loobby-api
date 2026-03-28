package com.jammes.loobby.events.dto

import com.jammes.loobby.events.model.EventType
import jakarta.validation.constraints.FutureOrPresent
import jakarta.validation.constraints.Size
import java.time.Instant

data class UpdateEventRequest(

    @field:Size(min = 1, max = 255)
    val name: String? = null,

    val description: String? = null,

    /**
     * Se informado, deve ser uma data futura.
     * Se não informado, mantém o valor atual.
     */
    @field:FutureOrPresent
    val scheduledDatetime: Instant? = null,

    /**
     * Detalhes específicos do tipo — só são aplicados se o evento
     * já for do tipo correspondente. Não é possível trocar o eventType.
     */
    val gameplay: GameplayEventDetailsRequest? = null,
    val sport: SportEventDetailsRequest? = null,

    /**
     * Flag explícita para limpar a descrição (setar null).
     * Se false/null, o campo description=null significa "não alterar".
     * Se true, a descrição será removida.
     */
    val clearDescription: Boolean = false
)