package app.loobby.games.service

/**
 * Lançada quando o RAWG está indisponível (timeout, 5xx, rate-limit) ou quando o
 * circuit breaker está aberto. O [GameService] captura e cai no fallback de cache.
 */
class RawgUnavailableException(message: String, cause: Throwable? = null) : RuntimeException(message, cause)
