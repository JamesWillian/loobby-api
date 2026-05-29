package app.loobby.games.service

import app.loobby.games.config.RawgProperties
import com.fasterxml.jackson.databind.JsonNode
import org.slf4j.LoggerFactory
import org.springframework.http.client.SimpleClientHttpRequestFactory
import org.springframework.stereotype.Component
import org.springframework.web.client.RestClient
import org.springframework.web.client.RestClientException
import org.springframework.web.client.HttpClientErrorException
import java.time.Instant
import java.util.concurrent.atomic.AtomicInteger

/**
 * Isola todo o HTTP do RAWG. Inclui um circuit breaker in-memory simples: se o RAWG
 * falhar [RawgProperties.circuitFailureThreshold] vezes seguidas, as chamadas são puladas
 * por [RawgProperties.circuitOpenMinutes] minutos para não engasgar a aplicação inteira.
 *
 * A API key vai na query string (?key=...). Como não há interceptor de logging configurado
 * no RestClient, a key não aparece em logs de request.
 */
@Component
class RawgClient(
    private val props: RawgProperties,
) {
    private val log = LoggerFactory.getLogger(RawgClient::class.java)

    private val restClient: RestClient = RestClient.builder()
        .baseUrl(props.baseUrl)
        .requestFactory(SimpleClientHttpRequestFactory().apply {
            setConnectTimeout(props.connectTimeoutMs)
            setReadTimeout(props.readTimeoutMs)
        })
        .build()

    // --- Circuit breaker ---
    private val consecutiveFailures = AtomicInteger(0)

    @Volatile
    private var openUntil: Instant? = null

    private fun ensureCircuitClosed() {
        val until = openUntil
        if (until != null && Instant.now().isBefore(until)) {
            throw RawgUnavailableException("RAWG circuit breaker aberto até $until")
        }
    }

    private fun recordSuccess() {
        consecutiveFailures.set(0)
        openUntil = null
    }

    private fun recordFailure() {
        val failures = consecutiveFailures.incrementAndGet()
        if (failures >= props.circuitFailureThreshold) {
            openUntil = Instant.now().plusSeconds(props.circuitOpenMinutes * 60)
            log.warn("RAWG circuit breaker aberto após {} falhas consecutivas até {}", failures, openUntil)
        }
    }

    /** GET /games?search=...&page=...  -> JsonNode da resposta completa (inclui "results"). */
    fun searchGames(query: String, page: Int): JsonNode {
        ensureCircuitClosed()
        return try {
            val node = restClient.get()
                .uri { b ->
                    b.path("/games")
                        .queryParam("key", props.apiKey)
                        .queryParam("search", query)
                        .queryParam("page", page)
                        .queryParam("page_size", props.searchPageSize)
                        .build()
                }
                .retrieve()
                .body(JsonNode::class.java)
                ?: throw RawgUnavailableException("RAWG /games retornou corpo vazio")
            recordSuccess()
            node
        } catch (e: RestClientException) {
            recordFailure()
            throw RawgUnavailableException("Falha ao buscar jogos no RAWG: ${e.message}", e)
        }
    }

    /** GET /games/{id} -> JsonNode, ou null se 404 (jogo não existe no RAWG). */
    fun getGame(id: String): JsonNode? {
        ensureCircuitClosed()
        return try {
            val node = restClient.get()
                .uri { b ->
                    b.path("/games/{id}")
                        .queryParam("key", props.apiKey)
                        .build(id)
                }
                .retrieve()
                .body(JsonNode::class.java)
            recordSuccess()
            node
        } catch (e: HttpClientErrorException.NotFound) {
            recordSuccess() // 404 é uma resposta válida do RAWG, não uma falha de infra
            null
        } catch (e: RestClientException) {
            recordFailure()
            throw RawgUnavailableException("Falha ao buscar jogo $id no RAWG: ${e.message}", e)
        }
    }
}
