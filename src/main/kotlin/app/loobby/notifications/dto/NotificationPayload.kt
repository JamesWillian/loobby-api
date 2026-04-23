package app.loobby.notifications.dto

import app.loobby.notifications.model.NotificationType

/**
 * Payload interno passado ao FcmSender. O mapa [data] vira os dados extras
 * no push, usados pelo app cliente para deep link / telemetria.
 *
 * Convenções de chaves em [data]:
 *  - "eventId"  → UUID do evento quando houver
 *  - "groupId"  → UUID do grupo quando houver
 *  - "actorId"  → UUID do usuário que originou a ação (quando houver)
 */
data class NotificationPayload(
    val title: String,
    val body: String,
    val type: NotificationType,
    val data: Map<String, String> = emptyMap()
)
