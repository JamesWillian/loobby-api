package app.loobby.games.model

import jakarta.persistence.Column
import jakarta.persistence.Entity
import jakarta.persistence.Id
import jakarta.persistence.Table
import org.hibernate.annotations.JdbcTypeCode
import org.hibernate.type.SqlTypes
import java.math.BigDecimal
import java.time.Instant
import java.time.LocalDate

/**
 * Espelho local (cache) de um jogo do catálogo RAWG.
 *
 * As colunas [genres], [platforms] e [rawPayload] são mapeadas como JSON (jsonb no Postgres):
 * o valor é uma String contendo JSON já serializado, gravada sem reescape.
 */
@Entity
@Table(name = "games")
open class GameEntity(

    @Id
    @Column(name = "id", nullable = false)
    var id: String,

    @Column(name = "slug")
    var slug: String? = null,

    @Column(name = "name", nullable = false)
    var name: String,

    @Column(name = "background_image")
    var backgroundImage: String? = null,

    @Column(name = "released")
    var released: LocalDate? = null,

    @Column(name = "rating")
    var rating: BigDecimal? = null,

    @Column(name = "metacritic")
    var metacritic: Int? = null,

    @Column(name = "description_raw", columnDefinition = "text")
    var descriptionRaw: String? = null,

    @JdbcTypeCode(SqlTypes.JSON)
    @Column(name = "genres", columnDefinition = "jsonb")
    var genres: String? = null,

    @JdbcTypeCode(SqlTypes.JSON)
    @Column(name = "platforms", columnDefinition = "jsonb")
    var platforms: String? = null,

    @JdbcTypeCode(SqlTypes.JSON)
    @Column(name = "raw_payload", columnDefinition = "jsonb")
    var rawPayload: String? = null,

    @Column(name = "cached_at", nullable = false)
    var cachedAt: Instant = Instant.now(),

    @Column(name = "refreshed_at", nullable = false)
    var refreshedAt: Instant = Instant.now(),
) {
    protected constructor() : this(
        id = "",
        name = "",
    )
}
