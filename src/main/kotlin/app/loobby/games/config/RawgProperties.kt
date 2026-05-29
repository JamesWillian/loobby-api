package app.loobby.games.config

import org.springframework.boot.context.properties.ConfigurationProperties
import org.springframework.context.annotation.Configuration

@Configuration
@ConfigurationProperties(prefix = "rawg")
class RawgProperties {
    /** Chave da API do RAWG (rawg.io/apidocs). Lida de env via application.yaml. */
    lateinit var apiKey: String

    /** Base URL da API do RAWG. */
    var baseUrl: String = "https://api.rawg.io/api"

    /** Timeout de conexão (ms). */
    var connectTimeoutMs: Int = 3000

    /** Timeout de leitura (ms). */
    var readTimeoutMs: Int = 5000

    /** Page size padrão da busca. */
    var searchPageSize: Int = 20

    /** TTL suave (dias): serve do cache e dispara refresh assíncrono (stale-while-revalidate). */
    var softTtlDays: Long = 7

    /** TTL rígido (dias): força refresh síncrono. */
    var hardTtlDays: Long = 30

    /** Circuit breaker: nº de falhas consecutivas antes de "abrir" o circuito. */
    var circuitFailureThreshold: Int = 5

    /** Circuit breaker: por quanto tempo (minutos) pular chamadas após abrir. */
    var circuitOpenMinutes: Long = 2
}
