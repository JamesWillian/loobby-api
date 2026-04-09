package app.loobby.events.teams.controller

import app.loobby.events.teams.dto.*
import app.loobby.events.teams.service.EventTeamService
import jakarta.validation.Valid
import org.springframework.security.core.annotation.AuthenticationPrincipal
import org.springframework.security.oauth2.jwt.Jwt
import org.springframework.web.bind.annotation.*
import java.util.UUID

@RestController
@RequestMapping("/events/{eventId}/teams")
class EventTeamController(
    private val eventTeamService: EventTeamService
) {

    // cria 1 time + jogadores
    @PostMapping
    fun createTeam(
        @PathVariable eventId: UUID,
        @Valid @RequestBody req: CreateTeamRequest,
        @AuthenticationPrincipal jwt: Jwt
    ): EventTeamResponse {
        return eventTeamService.createTeam(eventId, req)
    }

    // lista todos os times de um evento
    @GetMapping
    fun listTeams(
        @PathVariable eventId: UUID
    ): List<EventTeamResponse> {
        return eventTeamService.listTeams(eventId)
    }

    // atualiza nome / cor / ordem
    @PutMapping("/{teamId}")
    fun updateTeam(
        @PathVariable eventId: UUID,
        @PathVariable teamId: UUID,
        @RequestBody req: UpdateTeamRequest
    ): EventTeamResponse {
        return eventTeamService.updateTeam(eventId, teamId, req)
    }

    @DeleteMapping("/{teamId}")
    fun deleteTeam(
        @PathVariable eventId: UUID,
        @PathVariable teamId: UUID
    ) {
        return eventTeamService.deleteTeam(eventId, teamId)
    }

    // ---- players ----

    @PostMapping("/{teamId}/players")
    fun addPlayer(
        @PathVariable eventId: UUID,
        @PathVariable teamId: UUID,
        @RequestBody req: AddPlayerToTeamRequest
    ): EventTeamResponse {
        return eventTeamService.addPlayerToTeam(eventId, teamId, req)
    }

    @PutMapping("/{teamId}/players/{userId}")
    fun updatePlayer(
        @PathVariable eventId: UUID,
        @PathVariable teamId: UUID,
        @PathVariable userId: UUID,
        @RequestBody req: UpdateTeamPlayerRequest
    ): EventTeamResponse {
        return eventTeamService.updateTeamPlayer(eventId, teamId, userId, req)
    }

    @DeleteMapping("/{teamId}/players/{userId}")
    fun removePlayer(
        @PathVariable eventId: UUID,
        @PathVariable teamId: UUID,
        @PathVariable userId: UUID
    ): EventTeamResponse {
        return eventTeamService.removePlayerFromTeam(eventId, teamId, userId)
    }

    // ---- auto-generate ----

    @PostMapping("/auto-generate")
    fun autoGenerate(
        @PathVariable eventId: UUID,
        @RequestBody req: AutoGenerateTeamsRequest,
        @AuthenticationPrincipal jwt: Jwt
    ): List<EventTeamResponse> {
        return eventTeamService.autoGenerateTeams(eventId, req)
    }
}
