package app.loobby.notifications.service

import org.slf4j.LoggerFactory
import org.springframework.beans.factory.annotation.Value
import org.springframework.scheduling.TaskScheduler
import org.springframework.stereotype.Component
import java.time.Instant
import java.util.UUID
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.ScheduledFuture

/**
 * Agrupa notificações de RSVP peer dentro de uma janela de debounce (default 15 min)
 * para evitar que um destinatário receba 19 pushes seguidos quando muita gente confirma
 * presença em um grupo grande.
 *
 * Funcionamento:
 *  - A primeira confirmação agenda o disparo para "agora + debounce"
 *  - Cada confirmação adicional apenas acumula o nome do ator (até 2 nomes + contador)
 *  - Quando a janela fecha, executa o callback com (nomes, extrasCount) e limpa a chave
 *
 * ⚠️ IMPORTANTE: é in-memory. Um deploy/restart do backend perde as janelas em aberto
 * e pode disparar pushes já enfileirados. Isso é aceitável porque a janela é curta
 * e perder o agrupamento de ocasionais reinícios é melhor que montar infra de Redis
 * só para isso. Se escalar para múltiplas instâncias, trocar por Redis.
 */
@Component
class PeerRsvpDebouncer(
    private val taskScheduler: TaskScheduler,

    @Value("\${loobby.notifications.rsvp-peer-debounce-minutes}")
    private val debounceMinutes: Long
) {
    private val log = LoggerFactory.getLogger(javaClass)

    /** Chave: (eventId, recipientId). Representa 1 janela pendente por destinatário. */
    private data class Key(val eventId: UUID, val recipientId: UUID)

    private class Window(
        val firstActorNames: MutableList<String>,
        var extraActorsCount: Int,
        val actorIdsSeen: MutableSet<UUID>,
        var future: ScheduledFuture<*>?
    )

    private val windows = ConcurrentHashMap<Key, Window>()

    /**
     * Programa um push agrupado. Se já existe uma janela em aberto para este
     * (evento, destinatário), apenas acumula o nome do ator sem reagendar.
     *
     * @param onFlush callback que recebe (nomes, extrasCount). O NotificationService
     *                formata a mensagem e dispara o push.
     */
    fun schedule(
        eventId: UUID,
        recipientId: UUID,
        actorId: UUID,
        actorName: String,
        eventName: String,
        onFlush: (names: List<String>, extraCount: Int) -> Unit
    ) {
        val key = Key(eventId, recipientId)

        windows.compute(key) { _, existing ->
            if (existing == null) {
                // Primeira confirmação — cria janela e agenda flush
                val window = Window(
                    firstActorNames = mutableListOf(actorName),
                    extraActorsCount = 0,
                    actorIdsSeen = mutableSetOf(actorId),
                    future = null
                )
                val fireAt = Instant.now().plusSeconds(debounceMinutes * 60)
                window.future = taskScheduler.schedule({ flush(key, onFlush) }, fireAt)
                log.debug(
                    "PeerRsvp window opened event={} recipient={} fireAt={}",
                    eventId, recipientId, fireAt
                )
                window
            } else {
                // Janela já aberta — acumula, mas sem disparar duas vezes pelo mesmo ator
                if (existing.actorIdsSeen.add(actorId)) {
                    if (existing.firstActorNames.size < 2) {
                        existing.firstActorNames.add(actorName)
                    } else {
                        existing.extraActorsCount++
                    }
                }
                existing
            }
        }
    }

    private fun flush(key: Key, onFlush: (List<String>, Int) -> Unit) {
        val window = windows.remove(key) ?: return
        try {
            onFlush(window.firstActorNames.toList(), window.extraActorsCount)
        } catch (t: Throwable) {
            log.error(
                "PeerRsvp flush failed event={} recipient={}",
                key.eventId, key.recipientId, t
            )
        }
    }
}
