package app.loobby.events.teams.service

import app.loobby.events.model.RsvpStatus
import app.loobby.events.repo.EventRepository
import app.loobby.events.repo.EventRsvpRepository
import app.loobby.events.teams.dto.*
import app.loobby.events.teams.model.EventTeamEntity
import app.loobby.events.teams.model.TeamPlayerEntity
import app.loobby.events.teams.repo.EventTeamRepository
import app.loobby.events.teams.repo.TeamPlayerRepository
import app.loobby.users.repo.UsersRepository
import jakarta.persistence.EntityManager
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.stereotype.Service
import org.springframework.transaction.annotation.Transactional
import java.util.UUID
import kotlin.math.ceil
import kotlin.random.Random

@Service
class EventTeamService(
    private val eventRepository: EventRepository,
    private val eventRsvpRepository: EventRsvpRepository,
    private val eventTeamRepository: EventTeamRepository,
    private val teamPlayerRepository: TeamPlayerRepository,
    private val usersRepository: UsersRepository
) {

    private val teamColors = listOf(
        "#F44336", // vermelho
        "#2196F3", // azul
        "#4CAF50", // verde
        "#FF9800", // laranja
        "#9C27B0", // roxo
        "#009688", // teal
        "#3F51B5"  // indigo
    )

    // -------- CRUD BÁSICO --------

    @Transactional
    fun createTeam(eventId: UUID, req: CreateTeamRequest): EventTeamResponse {
        val event = eventRepository.findById(eventId)
            .orElseThrow { IllegalArgumentException("Event not found") }

        val nextOrder = req.order ?: run {
            val existing = eventTeamRepository.findByEventIdOrderByOrderIndexAsc(eventId)
            if (existing.isEmpty()) 1 else (existing.maxOf { it.orderIndex } + 1)
        }

        val team = eventTeamRepository.save(
            EventTeamEntity(
                eventId = event.id,
                orderIndex = nextOrder,
                name = req.name,
                color = req.color
            )
        )

        // cria players associados
        if (req.players.isNotEmpty()) {
            val users = usersRepository.findAllById(req.players.map { it.userId }.toSet())
            val usersById = users.associateBy { it.id }

            val entities = req.players.mapNotNull { p ->
                val user = usersById[p.userId] ?: return@mapNotNull null
                TeamPlayerEntity(
                    teamId = team.id,
                    userId = user.id,
                    role = p.role
                )
            }

            teamPlayerRepository.saveAll(entities)
        }

        return buildTeamResponse(team)
    }

    fun listTeams(eventId: UUID): List<EventTeamResponse> {
        if (!eventRepository.existsById(eventId)) {
            throw IllegalArgumentException("Event not found")
        }

        val teams = eventTeamRepository.findByEventIdOrderByOrderIndexAsc(eventId)
        if (teams.isEmpty()) return emptyList()

        return buildTeamResponses(teams)
    }

    @Transactional
    fun updateTeam(eventId: UUID, teamId: UUID, req: UpdateTeamRequest): EventTeamResponse {
        val team = eventTeamRepository.findById(teamId)
            .orElseThrow { IllegalArgumentException("Team not found") }

        if (team.eventId != eventId) {
            throw IllegalArgumentException("Team does not belong to this event")
        }

        req.name?.let { team.name = it }
        req.color?.let { team.color = it }
        req.order?.let { team.orderIndex = it }

        val saved = eventTeamRepository.save(team)
        return buildTeamResponse(saved)
    }

    @Transactional
    fun deleteTeam(eventId: UUID, teamId: UUID) {
        val team = eventTeamRepository.findById(teamId)
            .orElseThrow { IllegalArgumentException("Team not found") }

        if (team.eventId != eventId) {
            throw IllegalArgumentException("Team does not belong to this event")
        }

        // on delete cascade no banco remove os players
        eventTeamRepository.delete(team)
    }

    // -------- PLAYERS --------

    @Transactional
    fun addPlayerToTeam(eventId: UUID, teamId: UUID, req: AddPlayerToTeamRequest): EventTeamResponse {
        val team = eventTeamRepository.findById(teamId)
            .orElseThrow { IllegalArgumentException("Team not found") }

        if (team.eventId != eventId) {
            throw IllegalArgumentException("Team does not belong to this event")
        }

        val user = usersRepository.findById(req.userId)
            .orElseThrow { IllegalArgumentException("User not found") }

        // PK (team_id, user_id) garante que não duplica
        teamPlayerRepository.save(
            TeamPlayerEntity(
                teamId = team.id,
                userId = user.id,
                role = req.role
            )
        )

        return buildTeamResponse(team)
    }

    @Transactional
    fun updateTeamPlayer(eventId: UUID, teamId: UUID, userId: UUID, req: UpdateTeamPlayerRequest): EventTeamResponse {
        val team = eventTeamRepository.findById(teamId)
            .orElseThrow { IllegalArgumentException("Team not found") }

        if (team.eventId != eventId) {
            throw IllegalArgumentException("Team does not belong to this event")
        }

        var player = teamPlayerRepository.findByTeamId(teamId)
            .firstOrNull { it.userId == userId }
            ?: throw IllegalArgumentException("Player not found in this team")

        // mover de time
        if (req.newTeamId != null && req.newTeamId != teamId) {
            val newTeam = eventTeamRepository.findById(req.newTeamId)
                .orElseThrow { IllegalArgumentException("Target team not found") }

            if (newTeam.eventId != eventId) {
                throw IllegalArgumentException("Target team does not belong to this event")
            }

            // remove do time atual, adiciona no novo
            teamPlayerRepository.deleteByTeamIdAndUserId(teamId, userId)
            player = TeamPlayerEntity(
                teamId = newTeam.id,
                userId = userId,
                role = req.role ?: player.role
            )
            teamPlayerRepository.save(player)

            return buildTeamResponse(newTeam)
        }

        // só atualizar role
        req.role?.let { player.role = it }
        teamPlayerRepository.save(player)

        return buildTeamResponse(team)
    }

    @Transactional
    fun removePlayerFromTeam(eventId: UUID, teamId: UUID, userId: UUID): EventTeamResponse {
        val team = eventTeamRepository.findById(teamId)
            .orElseThrow { IllegalArgumentException("Team not found") }

        if (team.eventId != eventId) {
            throw IllegalArgumentException("Team does not belong to this event")
        }

        teamPlayerRepository.deleteByTeamIdAndUserId(teamId, userId)

        return buildTeamResponse(team)
    }

    // -------- AUTO-GERAR TIMES --------

    @Autowired
    private lateinit var entityManager: EntityManager

    /**
     * Gera times automaticamente:
     * - Usa apenas RSVPs YES como jogadores distribuídos em times numéricos.
     * - RSVPs RESERVE vão para um time "Reserva" (se existirem).
     * - Apaga times existentes do evento antes de gerar novos.
     */
    @Transactional
    fun autoGenerateTeams(eventId: UUID, req: AutoGenerateTeamsRequest): List<EventTeamResponse> {
        val event = eventRepository.findById(eventId)
            .orElseThrow { IllegalArgumentException("Event not found") }

        val rsvps = eventRsvpRepository.findByEventIdOrderByCreatedAtAsc(eventId)

        val confirmedIds = rsvps.filter { it.status == RsvpStatus.YES }.map { it.userId }
        val reserveIds = rsvps.filter { it.status == RsvpStatus.RESERVE }.map { it.userId }

        // limpa times atuais do evento (cascade remove players)
        eventTeamRepository.deleteByEventId(eventId)

        entityManager.flush()   // força o DELETE ir pro banco agora
        entityManager.clear()   // limpa o contexto de persistência

        if (confirmedIds.isEmpty() && reserveIds.isEmpty()) {
            return emptyList()
        }

        val shuffled = confirmedIds.shuffled(Random.Default)

        val teamCount = req.teamCount ?: run {
            val targetSize = req.teamSize ?: 5
            if (targetSize <= 0) 1
            else maxOf(1, ceil(shuffled.size.toDouble() / targetSize.toDouble()).toInt())
        }

        val createdTeams = mutableListOf<EventTeamEntity>()

        // cria times numerados
        if (shuffled.isNotEmpty()) {
            val chunkSize = ceil(shuffled.size.toDouble() / teamCount.toDouble()).toInt()
            val chunks = shuffled.chunked(chunkSize)

            chunks.forEachIndexed { index, players ->
                if (players.isEmpty()) return@forEachIndexed

                val team = eventTeamRepository.save(
                    EventTeamEntity(
                        eventId = event.id,
                        orderIndex = index + 1,
                        name = "Time ${index + 1}",
                        color = teamColors[index % teamColors.size]
                    )
                )
                createdTeams.add(team)

                val entities = players.map { userId ->
                    TeamPlayerEntity(
                        teamId = team.id,
                        userId = userId,
                        role = null
                    )
                }
                teamPlayerRepository.saveAll(entities)
            }
        }

        // cria time Reserva
        if (reserveIds.isNotEmpty()) {
            val reservaTeam = eventTeamRepository.save(
                EventTeamEntity(
                    eventId = event.id,
                    orderIndex = createdTeams.size + 1,
                    name = "Reserva",
                    color = "#9E9E9E"
                )
            )
            createdTeams.add(reservaTeam)

            val reservaPlayers = reserveIds.map { userId ->
                TeamPlayerEntity(
                    teamId = reservaTeam.id,
                    userId = userId,
                    role = null
                )
            }
            teamPlayerRepository.saveAll(reservaPlayers)
        }

        return buildTeamResponses(createdTeams)
    }

    // -------- HELPERS --------

    private fun buildTeamResponses(teams: List<EventTeamEntity>): List<EventTeamResponse> {
        val teamIds = teams.map { it.id }
        val players = teamPlayerRepository.findByTeamIdIn(teamIds)

        val users = usersRepository.findAllById(players.map { it.userId }.toSet())
        val usersById = users.associateBy { it.id }

        val playersByTeam = players.groupBy { it.teamId }

        return teams.map { team ->
            val teamPlayers = playersByTeam[team.id].orEmpty().mapNotNull { tp ->
                val user = usersById[tp.userId] ?: return@mapNotNull null

                TeamPlayerResponse(
                    userId = user.id,
                    role = tp.role,
                    username = user.username,
                    displayname = user.displayname,
                    avatarUrl = user.avatarUrl
                )
            }

            EventTeamResponse(
                id = team.id,
                eventId = team.eventId,
                order = team.orderIndex,
                name = team.name,
                color = team.color,
                players = teamPlayers
            )
        }.sortedBy { it.order }
    }

    private fun buildTeamResponse(team: EventTeamEntity): EventTeamResponse {
        return buildTeamResponses(listOf(team)).first()
    }
}
