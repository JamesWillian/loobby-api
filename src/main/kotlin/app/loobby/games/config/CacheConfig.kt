package app.loobby.games.config

import com.github.benmanes.caffeine.cache.Caffeine
import org.springframework.cache.CacheManager
import org.springframework.cache.annotation.EnableCaching
import org.springframework.cache.caffeine.CaffeineCacheManager
import org.springframework.context.annotation.Bean
import org.springframework.context.annotation.Configuration
import java.util.concurrent.TimeUnit

/**
 * Cache de busca por query exata (Caffeine, in-memory). Reduz chamadas ao RAWG e protege
 * a quota do plano gratuito. TTL curto porque a ordem dos resultados de busca muda com frequência.
 */
@Configuration
@EnableCaching
class CacheConfig {

    @Bean
    fun cacheManager(): CacheManager {
        val manager = CaffeineCacheManager("gameSearch")
        manager.setCaffeine(
            Caffeine.newBuilder()
                .expireAfterWrite(10, TimeUnit.MINUTES)
                .maximumSize(500)
        )
        return manager
    }
}
