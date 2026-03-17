package com.jammes.loobby.events.service

import com.jammes.loobby.events.dto.CreateEventRequest
import com.jammes.loobby.events.dto.EventResponse
import com.jammes.loobby.events.dto.GameplayEventDetailsResponse
import com.jammes.loobby.events.dto.SportEventDetailsResponse
import com.jammes.loobby.events.model.EventEntity
import com.jammes.loobby.events.model.EventType
import com.jammes.loobby.events.model.GameplayEventEntity
import com.jammes.loobby.events.model.RsvpStatus
import com.jammes.loobby.events.model.SportEventEntity
import com.jammes.loobby.events.repo.EventRepository
import com.jammes.loobby.events.repo.EventRsvpRepository
import com.jammes.loobby.events.repo.GameplayEventRepository
import com.jammes.loobby.events.repo.SportEventRepository
import com.jammes.loobby.groups.repo.GroupRepository
import com.jammes.loobby.users.repo.UsersRepository
import org.springframework.stereotype.Service
import java.math.BigDecimal
import java.util.UUID

@Service
class EventService(
    private val eventRepository: EventRepository,
    private val groupRepository: GroupRepository,
    private val gameplayEventRepository: GameplayEventRepository,
    private val sportEventRepository: SportEventRepository,
    private val eventRsvpRepository: EventRsvpRepository,
    private val usersRepository: UsersRepository
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
        val events = eventRepository.findByGroupIdOrderByScheduledDatetimeAsc(groupId)
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

    private fun generateInviteCode(): String {
        var code: String
        do {
            code = (1..8)
                .map { (('A'..'Z') + ('0'..'9')).random() }
                .joinToString("")
                .chunked(4)
                .joinToString("-")
        } while (eventRepository.existsByInviteCode(code))
        return "L-$code"
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