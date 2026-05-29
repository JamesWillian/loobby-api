package app.loobby.games.repo

import app.loobby.games.model.GameEntity
import org.springframework.data.jpa.repository.JpaRepository

interface GameRepository : JpaRepository<GameEntity, String> {

    fun findBySlug(slug: String): GameEntity?
}
