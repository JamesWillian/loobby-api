package app.loobby.notifications.service

import app.loobby.notifications.dto.NotificationPayload
import app.loobby.notifications.model.DeviceTokenEntity
import app.loobby.notifications.repo.DeviceTokenRepository
import com.google.firebase.messaging.FirebaseMessaging
import com.google.firebase.messaging.FirebaseMessagingException
import com.google.firebase.messaging.MessagingErrorCode
import com.google.firebase.messaging.MulticastMessage
import com.google.firebase.messaging.Notification
import org.slf4j.LoggerFactory
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.beans.factory.annotation.Value
import org.springframework.stereotype.Service
import org.springframework.transaction.annotation.Transactional
import java.util.UUID

/**
 * Envia pushes via FCM para um ou mais usuários.
 *
 *  - Busca todos os `device_tokens` dos usuários-alvo
 *  - Envia em lote (multicast, máx 500 por call)
 *  - Remove tokens inválidos (UNREGISTERED / INVALID_ARGUMENT)
 *
 * Em modo `enabled=false` (dev sem Firebase), só loga o que enviaria.
 */
@Service
class FcmSender(
    private val deviceTokenRepository: DeviceTokenRepository,

    // Opcional porque o bean é null quando notifications estão desabilitadas
    @Autowired(required = false)
    private val firebaseMessaging: FirebaseMessaging?,

    @Value("\${loobby.notifications.enabled}")
    private val enabled: Boolean
) {
    private val log = LoggerFactory.getLogger(javaClass)

    fun sendToUser(userId: UUID, payload: NotificationPayload) {
        sendToUsers(listOf(userId), payload)
    }

    /**
     * Envia a mesma notificação para um conjunto de usuários.
     * Lê e deleta tokens inválidos dentro de transações curtas.
     */
    fun sendToUsers(userIds: Collection<UUID>, payload: NotificationPayload) {
        if (userIds.isEmpty()) return

        if (!enabled || firebaseMessaging == null) {
            log.info(
                "Notifications disabled — would send type={} title='{}' body='{}' to users={}",
                payload.type, payload.title, payload.body, userIds
            )
            return
        }

        val tokens = loadTokens(userIds)
        if (tokens.isEmpty()) {
            log.debug("No device tokens for users={}", userIds)
            return
        }

        // FCM multicast: máx 500 tokens por request
        tokens.chunked(500).forEach { chunk -> sendChunk(chunk, payload) }
    }

    @Transactional(readOnly = true)
    protected fun loadTokens(userIds: Collection<UUID>): List<DeviceTokenEntity> {
        return deviceTokenRepository.findAllByUserIdIn(userIds)
    }

    private fun sendChunk(tokens: List<DeviceTokenEntity>, payload: NotificationPayload) {
        val tokenStrings = tokens.map { it.token }

        val message = MulticastMessage.builder()
            .addAllTokens(tokenStrings)
            .setNotification(
                Notification.builder()
                    .setTitle(payload.title)
                    .setBody(payload.body)
                    .build()
            )
            .putData("type", payload.type.name)
            .putData("title", payload.title)
            .putData("body", payload.body)
            .apply { payload.data.forEach { (k, v) -> putData(k, v) } }
            .build()

        try {
            val response = firebaseMessaging!!.sendEachForMulticast(message)
            log.info(
                "FCM sent type={} success={} failure={}",
                payload.type, response.successCount, response.failureCount
            )

            if (response.failureCount > 0) {
                val invalidTokens = response.responses.mapIndexedNotNull { idx, resp ->
                    if (!resp.isSuccessful && isInvalidTokenError(resp.exception)) {
                        tokenStrings[idx]
                    } else null
                }
                if (invalidTokens.isNotEmpty()) {
                    log.info("Removing {} invalid FCM tokens", invalidTokens.size)
                    purgeInvalidTokens(invalidTokens)
                }
            }
        } catch (t: Throwable) {
            log.error("Failed to send FCM multicast (type={})", payload.type, t)
        }
    }

    @Transactional
    protected fun purgeInvalidTokens(tokens: Collection<String>) {
        deviceTokenRepository.deleteAllByTokenIn(tokens)
    }

    private fun isInvalidTokenError(e: FirebaseMessagingException?): Boolean {
        if (e == null) return false
        return e.messagingErrorCode == MessagingErrorCode.UNREGISTERED ||
                e.messagingErrorCode == MessagingErrorCode.INVALID_ARGUMENT
    }
}
