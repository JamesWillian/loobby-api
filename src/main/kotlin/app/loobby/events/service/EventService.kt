package app.loobby.events.service

import app.loobby.events.dto.CreateEventRequest
import app.loobby.events.dto.EventResponse
import app.loobby.events.dto.GameplayEventDetailsResponse
import app.loobby.events.dto.SportEventDetailsResponse
import app.loobby.events.dto.UpdateEventRequest
import app.loobby.events.model.EventEntity
import app.loobby.events.model.EventType
import app.loobby.events.model.GameplayEventEntity
import app.loobby.events.model.RsvpStatus
import app.loobby.events.model.SportEventEntity
import app.loobby.events.repo.EventRepository
import app.loobby.events.repo.EventRsvpRepository
import app.loobby.events.repo.GameplayEventRepository
import app.loobby.events.repo.SportEventRepository
import app.loobby.groups.repo.GroupRepository
import app.loobby.notifications.service.NotificationService
import app.loobby.users.repo.UsersRepository
import org.springframework.security.access.AccessDeniedException
import org.springframework.stereotype.Service
import org.springframework.transaction.annotation.Transactional
import java.math.BigDecimal
import java.util.UUID

@Service
class EventService(
    private val eventRepository: EventRepository,
    private val groupRepository: GroupRepository,
    private val gameplayEventRepository: GameplayEventRepository,
    private val sportEventRepository: SportEventRepository,
    private val eventRsvpRepository: EventRsvpRepository,
    private val usersRepository: UsersRepository,
    private val notificationService: NotificationService,
    private val gameService: app.loobby.games.service.GameService
) {

    fun createGroupEvent(
        ownerId: UUID,
        groupId: UUID,
        req: CreateEventRequest
    ): EventResponse {
        val group = groupRepository.findById(groupId)
            .orElseThrow { IllegalArgumentException("Group not found") }

        val inviteCode = generateInviteCode()

        val event = eventRepository.save(
            EventEntity(
                eventType = req.eventType,
                groupId = group.id,
                isInstant = false,
                ownerId = ownerId,
                scheduledDatetime = req.scheduledDatetime,
                name = req.name,
                description = req.description,
                inviteCode = inviteCode
            )
        )

        when (req.eventType) {
            EventType.GAMEPLAY -> {
                val details = req.gameplay
                    ?: throw IllegalArgumentException("Gameplay details are required for GAMEPLAY events")
                if (details.gameName.isBlank())
                    throw IllegalArgumentException("Gameplay event must have gameName")
                gameplayEventRepository.save(
                    GameplayEventEntity(eventId = event.id, gameId = details.gameId, gameName = details.gameName)
                )
                // Garante (em background) que o jogo está no cache local para detalhe futuro.
                details.gameId?.let { gameService.ensureCached(it) }
            }
            EventType.SPORT -> {
                val details = req.sport
                    ?: throw IllegalArgumentException("Sport details are required for SPORT events")
                if (details.durationMinutes <= 0)
                    throw IllegalArgumentException("durationMinutes must be > 0")
                sportEventRepository.save(
                    SportEventEntity(
                        eventId = event.id,
                        durationMinutes = details.durationMinutes,
                        arena = details.arena,
                        pricePerPlayer = details.pricePerPlayer ?: BigDecimal.ZERO,
                        maxPlayers = details.maxPlayers,
                        acceptReserve = details.acceptReserve ?: false
                    )
                )
            }
            else -> {}
        }

        // Caso 5: notifica todos os membros do grupo (exceto o criador)
        notificationService.onGroupEventCreated(event, ownerId)

        return buildResponse(event)
    }

    fun createInstantEvent(
        ownerId: UUID,
        req: CreateEventRequest
    ): EventResponse {
        val inviteCode = generateInviteCode()

        val event = eventRepository.save(
            EventEntity(
                eventType = req.eventType,
                groupId = null,
                isInstant = true,
                ownerId = ownerId,
                scheduledDatetime = req.scheduledDatetime,
                name = req.name,
                description = req.description,
                inviteCode = inviteCode
            )
        )

        when (req.eventType) {
            EventType.GAMEPLAY -> {
                val details = req.gameplay
                    ?: throw IllegalArgumentException("Gameplay details are required for GAMEPLAY events")
                if (details.gameName.isBlank())
                    throw IllegalArgumentException("Gameplay event must have gameName")
                gameplayEventRepository.save(
                    GameplayEventEntity(eventId = event.id, gameId = details.gameId, gameName = details.gameName)
                )
                // Garante (em background) que o jogo está no cache local para detalhe futuro.
                details.gameId?.let { gameService.ensureCached(it) }
            }
            EventType.SPORT -> {
                val details = req.sport
                    ?: throw IllegalArgumentException("Sport details are required for SPORT events")
                if (details.durationMinutes <= 0)
                    throw IllegalArgumentException("durationMinutes must be > 0")
                sportEventRepository.save(
                    SportEventEntity(
                        eventId = event.id,
                        durationMinutes = details.durationMinutes,
                        arena = details.arena,
                        pricePerPlayer = details.pricePerPlayer ?: BigDecimal.ZERO,
                        maxPlayers = details.maxPlayers,
                        acceptReserve = details.acceptReserve ?: false
                    )
                )
            }
            else -> {}
        }

        return buildResponse(event)
    }

    fun listGroupEvents(groupId: UUID, userId: UUID): List<EventResponse> {
        val events = eventRepository.findByGroupIdOrderByScheduledDatetimeDesc(groupId)
        if (events.isEmpty()) return emptyList()

        val eventIds = events.map { it.id }

        val rsvps = eventRsvpRepository.findByEventIdInAndUserId(eventIds, userId)
        val rsvpByEventId = rsvps.associateBy({ it.eventId }, { it.status })

        val allYesRsvps = eventRsvpRepository.findByEventIdInAndStatus(eventIds, RsvpStatus.YES)
        val yesByEventId = allYesRsvps.groupBy { it.eventId }

        val top5UserIds = yesByEventId.values
            .flatMap { rsvpList -> rsvpList.take(5).map { it.userId } }
            .toSet()
        val usersById = usersRepository.findAllById(top5UserIds).associateBy { it.id }

        return events.map { event ->
            val yesRsvps = yesByEventId[event.id] ?: emptyList()
            val avatars = yesRsvps.take(5)
                .map { usersById[it.userId]?.avatarUrl }
                .takeIf { it.isNotEmpty() }

            buildResponse(
                event = event,
                rsvpStatus = rsvpByEventId[event.id],
                confirmedCount = yesRsvps.size,
                confirmedAvatars = avatars
            )
        }
    }

    // Agora recebe o userId para incluir o rsvpStatus do usuário na resposta
    fun getEventById(eventId: UUID, userId: UUID): EventResponse {
        val event = eventRepository.findById(eventId)
            .orElseThrow { IllegalArgumentException("Event not found") }

        val rsvpStatus = eventRsvpRepository
            .findByEventIdAndUserId(eventId, userId)
            ?.status

        val yesRsvps = eventRsvpRepository.findByEventIdInAndStatus(listOf(eventId), RsvpStatus.YES)
        val userIds = yesRsvps.take(5).map { it.userId }.toSet()
        val usersById = usersRepository.findAllById(userIds).associateBy { it.id }
        val avatars = yesRsvps.take(5).map { usersById[it.userId]?.avatarUrl }.takeIf { it.isNotEmpty() }

        return buildResponse(
            event = event,
            rsvpStatus = rsvpStatus,
            confirmedCount = yesRsvps.size,
            confirmedAvatars = avatars
        )
    }

    // ========================= getByInviteCode =========================

    /**
     * Busca um evento pelo invite code.
     * Retorna o evento com o rsvpStatus do usuário autenticado,
     * seguindo a mesma convenção de getEventById.
     */
    fun getByInviteCode(code: String, userId: UUID): EventResponse {
        val event = eventRepository.findByInviteCode(code)
            ?: throw IllegalStateException("Invalid Invite Code")

        val rsvpStatus = eventRsvpRepository
            .findByEventIdAndUserId(event.id, userId)
            ?.status

        val yesRsvps = eventRsvpRepository.findByEventIdInAndStatus(listOf(event.id), RsvpStatus.YES)
        val userIds = yesRsvps.take(5).map { it.userId }.toSet()
        val usersById = usersRepository.findAllById(userIds).associateBy { it.id }
        val avatars = yesRsvps.take(5).map { usersById[it.userId]?.avatarUrl }.takeIf { it.isNotEmpty() }

        return buildResponse(
            event = event,
            rsvpStatus = rsvpStatus,
            confirmedCount = yesRsvps.size,
            confirmedAvatars = avatars
        )
    }

    // ========================= updateEvent =========================

    /**
     * Atualiza um evento existente.
     *
     * Regras:
     *  - Apenas o criador do evento OU o dono do grupo (se houver) podem alterar.
     *  - Não é permitido alterar: eventType, groupId, isInstant.
     *  - Detalhes específicos (gameplay/sport) só são atualizados se o tipo bater.
     */
    @Transactional
    fun updateEvent(
        userId: UUID,
        eventId: UUID,
        req: UpdateEventRequest
    ): EventResponse {
        val event = eventRepository.findById(eventId)
            .orElseThrow { IllegalArgumentException("Event not found") }

        // ---------- autorização ----------
        assertCanManageEvent(userId, event)

        // ---------- campos gerais ----------
        req.name?.let { event.name = it }

        if (req.clearDescription) {
            event.description = null
        } else {
            req.description?.let { event.description = it }
        }

        req.scheduledDatetime?.let { event.scheduledDatetime = it }

        eventRepository.save(event)

        // ---------- detalhes específicos ----------
        when (event.eventType) {
            EventType.GAMEPLAY -> {
                req.gameplay?.let { details ->
                    val gameplay = gameplayEventRepository.findByEventId(event.id)
                        ?: throw IllegalStateException("Gameplay details not found for event ${event.id}")
                    if (details.gameName.isNotBlank()) gameplay.gameName = details.gameName
                    details.gameId?.let { gameplay.gameId = it }
                    gameplayEventRepository.save(gameplay)
                    details.gameId?.let { gameService.ensureCached(it) }
                }
            }
            EventType.SPORT -> {
                req.sport?.let { details ->
                    val sport = sportEventRepository.findByEventId(event.id)
                        ?: throw IllegalStateException("Sport details not found for event ${event.id}")
                    if (details.durationMinutes > 0) sport.durationMinutes = details.durationMinutes
                    details.arena?.let { sport.arena = it }
                    details.pricePerPlayer?.let { sport.pricePerPlayer = it }
                    details.maxPlayers?.let { sport.maxPlayers = it }
                    details.acceptReserve?.let { sport.acceptReserve = it }
                    sportEventRepository.save(sport)
                }
            }
            else -> {}
        }

        return buildResponse(event)
    }

    // ========================= deleteEvent =========================

    /**
     * Exclui um evento
     *
     * Regras:
     *  - Apenas o criador do evento OU o dono do grupo (se houver) podem excluir.
     */
    @Transactional
    fun deleteEvent(userId: UUID, eventId: UUID) {
        val event = eventRepository.findById(eventId)
            .orElseThrow { IllegalArgumentException("Event not found") }

        // ---------- autorização ----------
        assertCanManageEvent(userId, event)

        eventRepository.delete(event)
    }

    // ========================= helper de autorização =========================

    /**
     * Verifica se o usuário é o criador do evento OU o dono do grupo associado.
     * Lança AccessDeniedException se não tiver permissão.
     */
    private fun assertCanManageEvent(userId: UUID, event: EventEntity) {
        val isEventOwner = event.ownerId == userId
        val isGroupOwner = event.groupId?.let { gid ->
            groupRepository.findById(gid).map { it.ownerId == userId }.orElse(false)
        } ?: false

        if (!isEventOwner && !isGroupOwner) {
            throw AccessDeniedException(
                "Only the event creator or the group owner can manage this event"
            )
        }
    }

    private fun generateInviteCode(): String {
        var code: String
        do {
            code = (1..8)
                .map { (('A'..'Z') + ('0'..'9')).random() }
                .joinToString("")
                .chunked(4)
                .joinToString("-")
        } while (eventRepository.existsByInviteCode(code))
        return "$-$code"
    }

    private fun buildResponse(
        event: EventEntity,
        rsvpStatus: RsvpStatus? = null,
        confirmedCount: Int = 0,
        confirmedAvatars: List<String?>? = null
    ): EventResponse {
        val gameplayDetails = if (event.eventType == EventType.GAMEPLAY) {
            gameplayEventRepository.findByEventId(event.id)?.let {
                GameplayEventDetailsResponse(gameId = it.gameId, gameName = it.gameName)
            }
        } else null

        val sportDetails = if (event.eventType == EventType.SPORT) {
            sportEventRepository.findByEventId(event.id)?.let {
                SportEventDetailsResponse(
                    durationMinutes = it.durationMinutes,
                    arena = it.arena,
                    pricePerPlayer = it.pricePerPlayer,
                    maxPlayers = it.maxPlayers,
                    acceptReserve = it.acceptReserve
                )
            }
        } else null

        return EventResponse(
            id = event.id,
            eventType = event.eventType,
            groupId = event.groupId,
            isInstant = event.isInstant,
            ownerId = event.ownerId,
            scheduledDatetime = event.scheduledDatetime,
            name = event.name,
            description = event.description,
            inviteCode = event.inviteCode,
            createdAt = event.createdAt,
            rsvpStatus = rsvpStatus,
            confirmedCount = confirmedCount,
            confirmedAvatars = confirmedAvatars,
            gameplay = gameplayDetails,
            sport = sportDetails,
        )
    }
}