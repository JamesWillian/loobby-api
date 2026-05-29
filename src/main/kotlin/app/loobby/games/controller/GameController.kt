package app.loobby.games.controller

import app.loobby.games.dto.GameDetailsResponse
import app.loobby.games.dto.GameSearchResponse
import app.loobby.games.service.GameService
import jakarta.validation.constraints.NotBlank
import org.springframework.validation.annotation.Validated
import org.springframework.web.bind.annotation.GetMapping
import org.springframework.web.bind.annotation.PathVariable
import org.springframework.web.bind.annotation.RequestMapping
import org.springframework.web.bind.annotation.RequestParam
import org.springframework.web.bind.annotation.RestController

/**
 * Proxy autenticado para o catálogo RAWG. O app mobile fala SOMENTE com estes endpoints,
 * nunca direto com o RAWG (a API key fica no servidor). Rotas exigem JWT por padrão
 * (SecurityConfig: anyRequest().authenticated()).
 */
@RestController
@RequestMapping("/games")
@Validated
class GameController(
    private val gameService: GameService,
) {

    @GetMapping("/search")
    fun search(
        @RequestParam("q") @NotBlank query: String,
        @RequestParam(name = "page", defaultValue = "1") page: Int,
    ): GameSearchResponse = gameService.search(query, page)

    @GetMapping("/{id}")
    fun details(@PathVariable id: String): GameDetailsResponse =
        gameService.getDetails(id)
}
