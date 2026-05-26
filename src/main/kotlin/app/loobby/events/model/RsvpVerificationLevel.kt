package app.loobby.events.model

/**
 * Quão confiável é a identidade por trás de uma confirmação de presença.
 *
 * Persistido como texto em event_rsvps.verification_level via @Enumerated(EnumType.STRING).
 */
enum class RsvpVerificationLevel {
    /** Posse do telefone provada via OTP (Firebase Phone Auth) antes da criação da RSVP. */
    PHONE_VERIFIED,

    /** Usuário autenticado no app (e-mail, Google, etc.). */
    APP_AUTHENTICATED
}
