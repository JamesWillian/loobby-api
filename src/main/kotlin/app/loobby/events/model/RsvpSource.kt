package app.loobby.events.model

/**
 * Por onde a confirmação de presença entrou no sistema.
 *
 * Persistido como texto em event_rsvps.source via @Enumerated(EnumType.STRING).
 */
enum class RsvpSource {
    /** Fluxo padrão: usuário autenticado no app. */
    APP,

    /** Confirmação criada via URL pública (link assinado compartilhado em grupo). */
    WEB_LINK
}
