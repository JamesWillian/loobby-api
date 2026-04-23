package app.loobby.notifications.model

/**
 * Tipos de notificação. IMPORTANTE: o nome do enum é persistido em
 * [app.loobby.notifications.model.NotificationDispatchLogEntity.notificationType]
 * como chave de idempotência diária — NUNCA renomear valores existentes sem migração de dados.
 */
enum class NotificationType {
    /** Caso 1: alguém confirmou presença em evento que também confirmei (agrupado com debounce) */
    RSVP_CONFIRMED_BY_PEER,

    /** Caso 2: evento hoje e ainda não confirmei presença */
    EVENT_TODAY_PENDING_RSVP,

    /** Caso 3: lembrete de evento confirmado a ~X horas do horário */
    EVENT_REMINDER_HOURS_BEFORE,

    /** Caso 4: pagamento pendente (dispara 1x por dia via dispatch_log) */
    PAYMENT_DUE_DAILY,

    /** Caso 5: novo evento criado em grupo que participo */
    NEW_GROUP_EVENT
}
