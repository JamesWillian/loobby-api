package app.loobby.events.service

import org.springframework.http.HttpStatus
import org.springframework.stereotype.Component
import org.springframework.web.server.ResponseStatusException
import java.time.Duration
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.locks.ReentrantLock

/**
 * Token-bucket rate limiter em memória para os endpoints públicos de RSVP.
 *
 * Política atual:
 *  - Por IP:       10 chamadas / minuto
 *  - Por link:     30 chamadas / hora
 *  - Por telefone: 1 chamada a cada 60s (anti-spam de SMS no Firebase)
 *
 * Os dois primeiros buckets se aplicam tanto a /start quanto a /confirm,
 * já que cada chamada implica trabalho no backend (validação, query no banco).
 * O bucket por telefone só se aplica a /start, porque é onde o cliente
 * provoca o envio de SMS pelo Firebase.
 *
 * Aderência a SOLID:
 *  - SRP: este componente só faz rate-limit. Nenhuma decisão de negócio aqui.
 *  - OCP: mudar limites é alterar constantes; trocar o algoritmo (token bucket
 *    → sliding window, Redis) é trocar a implementação sem mexer nos chamadores.
 *
 * Limitações conhecidas:
 *  - In-memory: estado não é compartilhado entre instâncias. Quando escalar
 *    horizontalmente, migrar para Redis / Bucket4j-Hazelcast.
 *  - Buckets não são desalocados. Para tráfego significativo, considerar
 *    Caffeine com expireAfterAccess.
 */
@Component
class PublicRsvpRateLimiter {

    private val ipBuckets = ConcurrentHashMap<String, TokenBucket>()
    private val linkBuckets = ConcurrentHashMap<String, TokenBucket>()
    private val phoneBuckets = ConcurrentHashMap<String, TokenBucket>()

    /**
     * Aplica os limites por IP e por link público. Levanta 429 se algum estourar.
     * Quando o link aceita mas o IP não, devolve os tokens consumidos no link
     * para não penalizar quem está usando o mesmo link de outro IP válido.
     */
    fun checkRequest(ip: String, linkToken: String) {
        val linkBucket = linkBuckets.computeIfAbsent(linkToken) { newLinkBucket() }
        if (!linkBucket.tryConsume(1)) {
            throw tooManyRequests("rate limit exceeded for this link")
        }

        val ipBucket = ipBuckets.computeIfAbsent(ip) { newIpBucket() }
        if (!ipBucket.tryConsume(1)) {
            linkBucket.returnTokens(1)
            throw tooManyRequests("rate limit exceeded for this client")
        }
    }

    /**
     * Cooldown global por número de telefone. Garante que o Firebase não
     * receba bursts de SMS para o mesmo número (cada SMS tem custo).
     */
    fun checkPhoneCooldown(phoneE164: String) {
        val bucket = phoneBuckets.computeIfAbsent(phoneE164) { newPhoneBucket() }
        if (!bucket.tryConsume(1)) {
            throw tooManyRequests("aguarde um momento antes de pedir outro código SMS")
        }
    }

    // -------------------------------------------------------------------------
    // Factory de buckets — constantes ficam centralizadas aqui.
    // -------------------------------------------------------------------------

    private fun newIpBucket() = TokenBucket(
        capacity = IP_CAPACITY,
        refillTokens = IP_CAPACITY,
        refillIntervalNanos = Duration.ofMinutes(1).toNanos()
    )

    private fun newLinkBucket() = TokenBucket(
        capacity = LINK_CAPACITY,
        refillTokens = LINK_CAPACITY,
        refillIntervalNanos = Duration.ofHours(1).toNanos()
    )

    private fun newPhoneBucket() = TokenBucket(
        capacity = PHONE_CAPACITY,
        refillTokens = PHONE_CAPACITY,
        refillIntervalNanos = Duration.ofMinutes(3).toNanos()
    )

    private fun tooManyRequests(message: String) =
        ResponseStatusException(HttpStatus.TOO_MANY_REQUESTS, message)

    // -------------------------------------------------------------------------
    // Token bucket clássico, thread-safe.
    // -------------------------------------------------------------------------
    private class TokenBucket(
        private val capacity: Long,
        private val refillTokens: Long,
        private val refillIntervalNanos: Long
    ) {
        private var tokens: Long = capacity
        private var lastRefill: Long = System.nanoTime()
        private val lock = ReentrantLock()

        fun tryConsume(n: Long): Boolean {
            lock.lock()
            try {
                refill()
                if (tokens >= n) {
                    tokens -= n
                    return true
                }
                return false
            } finally {
                lock.unlock()
            }
        }

        fun returnTokens(n: Long) {
            lock.lock()
            try {
                tokens = (tokens + n).coerceAtMost(capacity)
            } finally {
                lock.unlock()
            }
        }

        private fun refill() {
            val now = System.nanoTime()
            val elapsed = now - lastRefill
            if (elapsed >= refillIntervalNanos) {
                val intervals = elapsed / refillIntervalNanos
                tokens = (tokens + intervals * refillTokens).coerceAtMost(capacity)
                lastRefill += intervals * refillIntervalNanos
            }
        }
    }

    companion object {
        private const val IP_CAPACITY = 10L
        private const val LINK_CAPACITY = 30L
        private const val PHONE_CAPACITY = 1L
    }
}
