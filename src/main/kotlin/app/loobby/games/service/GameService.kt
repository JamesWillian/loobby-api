package app.loobby.games.service

import app.loobby.games.config.RawgProperties
import app.loobby.games.dto.GameDetailsResponse
import app.loobby.games.dto.GameSearchResponse
import app.loobby.games.dto.GameSummaryResponse
import app.loobby.games.model.GameEntity
import app.loobby.games.repo.GameRepository
import com.fasterxml.jackson.databind.JsonNode
import com.fasterxml.jackson.databind.ObjectMapper
import org.slf4j.LoggerFactory
import org.springframework.beans.factory.annotation.Qualifier
import org.springframework.cache.annotation.Cacheable
import org.springframework.core.task.AsyncTaskExecutor
import org.springframework.http.HttpStatus
import org.springframework.stereotype.Service
import org.springframework.web.server.ResponseStatusException
import java.time.Duration
import java.time.Instant
import java.time.LocalDate

/**
 * Read-through do catálogo RAWG com cache local (tabela games) e estratégia de freshness:
 *  - soft TTL: serve do cache na hora e dispara refresh assíncrono (stale-while-revalidate)
 *  - hard TTL: força refresh síncrono
 *  - falha no RAWG: devolve o cache mesmo vencido (fallback) em vez de tela vazia
 */
@Service
class GameService(
    private val gameRepository: GameRepository,
    private val rawgClient: RawgClient,
    private val props: RawgProperties,
    private val objectMapper: ObjectMapper,
    @Qualifier("applicationTaskExecutor") private val taskExecutor: AsyncTaskExecutor,
) {
    private val log = LoggerFactory.getLogger(GameService::class.java)

    private val softTtl: Duration get() = Duration.ofDays(props.softTtlDays)
    private val hardTtl: Duration get() = Duration.ofDays(props.hardTtlDays)

    /**
     * Busca no RAWG (proxy). Resultados são cacheados por query+page (Caffeine, ~10min) e
     * cada item é upsertado na tabela games para popular o cache organicamente.
     */
    @Cacheable("gameSearch", key = "#query.trim().toLowerCase() + ':' + #page")
    fun search(query: String, page: Int): GameSearchResponse {
        val node = rawgClient.searchGames(query, page)
        val results = node.path("results").mapNotNull { item ->
            runCatching { upsertFromSearch(item) }
                .onFailure { log.warn("Falha ao cachear item de busca do RAWG: {}", it.message) }
                .getOrNull()
        }
        return GameSearchResponse(
            page = page,
            count = node.path("count").asInt(results.size),
            results = results.map { it.toSummary() },
        )
    }

    /**
     * Detalhe de um jogo. Read-through com TTL e fallback de cache.
     */
    fun getDetails(id: String): GameDetailsResponse {
        val cached = gameRepository.findById(id).orElse(null)

        if (cached != null) {
            val age = Duration.between(cached.refreshedAt, Instant.now())
            val needsFull = cached.descriptionRaw == null
            return when {
                // Tela de detalhe precisa de descrição: se ainda não temos, busca completa agora.
                needsFull -> refreshSync(id) ?: cached.toDetails()
                age <= softTtl -> cached.toDetails()
                age <= hardTtl -> {
                    triggerAsyncRefresh(id) // stale-while-revalidate
                    cached.toDetails()
                }
                else -> refreshSync(id) ?: cached.toDetails()
            }
        }

        // Não está no cache -> precisa buscar.
        return refreshSync(id)
            ?: throw ResponseStatusException(HttpStatus.NOT_FOUND, "Jogo $id não encontrado")
    }

    /**
     * Garante que o jogo está no cache (chamado pelo EventService ao criar gameplay_event).
     * Best-effort e assíncrono: nunca bloqueia nem quebra a criação do evento.
     */
    fun ensureCached(id: String) {
        taskExecutor.execute {
            runCatching {
                val existing = gameRepository.findById(id).orElse(null)
                val stale = existing == null ||
                    Duration.between(existing.refreshedAt, Instant.now()) > hardTtl
                if (stale) refreshSync(id)
            }.onFailure { log.warn("ensureCached({}) falhou: {}", id, it.message) }
        }
    }

    private fun triggerAsyncRefresh(id: String) {
        taskExecutor.execute {
            runCatching { refreshSync(id) }
                .onFailure { log.warn("Refresh assíncrono de {} falhou: {}", id, it.message) }
        }
    }

    /**
     * Busca o detalhe completo no RAWG e atualiza o cache.
     * Retorna null em caso de falha do RAWG (deixa o caller decidir o fallback).
     */
    private fun refreshSync(id: String): GameDetailsResponse? {
        return try {
            val node = rawgClient.getGame(id) ?: return null
            upsertFromDetail(node).toDetails()
        } catch (e: RawgUnavailableException) {
            log.warn("RAWG indisponível ao buscar {}: {}", id, e.message)
            null
        }
    }

    // --- Upsert / mapeamento ---

    private fun upsertFromSearch(node: JsonNode): GameEntity {
        val id = node.path("id").asText()
        val entity = gameRepository.findById(id).orElseGet { GameEntity(id = id, name = "") }
        applyCommonFields(entity, node)
        // Busca não traz description_raw; preserva o que já existe.
        // Só grava raw_payload parcial se o registro for novo (não sobrescreve payload completo).
        if (entity.rawPayload == null) entity.rawPayload = node.toString()
        entity.refreshedAt = Instant.now()
        return gameRepository.save(entity)
    }

    private fun upsertFromDetail(node: JsonNode): GameEntity {
        val id = node.path("id").asText()
        val entity = gameRepository.findById(id).orElseGet { GameEntity(id = id, name = "") }
        applyCommonFields(entity, node)
        entity.descriptionRaw = node.path("description_raw").takeIf { it.isTextual }?.asText()
            ?: node.path("description").takeIf { it.isTextual }?.asText()
        entity.rawPayload = node.toString()
        entity.refreshedAt = Instant.now()
        return gameRepository.save(entity)
    }

    private fun applyCommonFields(entity: GameEntity, node: JsonNode) {
        node.path("name").takeIf { it.isTextual }?.let { entity.name = it.asText() }
        entity.slug = node.path("slug").takeIf { it.isTextual }?.asText()
        entity.backgroundImage = node.path("background_image").takeIf { it.isTextual }?.asText()
        entity.released = node.path("released").takeIf { it.isTextual }?.asText()?.let { parseDate(it) }
        entity.rating = node.path("rating").takeIf { it.isNumber }?.decimalValue()
        entity.metacritic = node.path("metacritic").takeIf { it.isInt }?.asInt()
        entity.genres = node.get("genres")?.takeIf { !it.isNull }?.toString()
        entity.platforms = node.get("platforms")?.takeIf { !it.isNull }?.toString()
    }

    private fun parseDate(value: String): LocalDate? =
        runCatching { LocalDate.parse(value) }.getOrNull()

    private fun parseJson(value: String?): JsonNode? =
        value?.let { runCatching { objectMapper.readTree(it) }.getOrNull() }

    private fun GameEntity.toSummary() = GameSummaryResponse(
        id = id,
        slug = slug,
        name = name,
        backgroundImage = backgroundImage,
        released = released?.toString(),
        rating = rating,
        metacritic = metacritic,
    )

    private fun GameEntity.toDetails() = GameDetailsResponse(
        id = id,
        slug = slug,
        name = name,
        backgroundImage = backgroundImage,
        released = released?.toString(),
        rating = rating,
        metacritic = metacritic,
        descriptionRaw = descriptionRaw,
        genres = parseJson(genres),
        platforms = parseJson(platforms),
    )
}
