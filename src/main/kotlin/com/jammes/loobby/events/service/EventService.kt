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
import org.springframework.stereotype.Service
import java.math.BigDecimal
import java.util.UUID

@Service
class EventService(
    private val eventRepository: EventRepository,
    private val groupRepository: GroupRepository,
    private val gameplayEventRepository: GameplayEventRepository,
    private val sportEventRepository: SportEventRepository,
    private val eventRsvpRepository: EventRsvpRepository
) {

    fun createGroupEvent(
        ownerId: UUID,
        groupId: UUID,
        req: CreateEventRequest
    ): EventResponse {
        // garante que o grupo existe
        val group = groupRepository.findById(groupId)
            .orElseThrow { IllegalArgumentException("Group not found") }

        // aqui daria pra validar se o owner é membro/owner do grupo
        // depois a gente pode enrijecer essa regra

        val inviteCode = generateInviteCode()

        val event = eventRepository.save(
            EventEntity(
                eventType = req.eventType,
                groupId = group.id,
                isInstant = false, // importante pro check constraint
                ownerId = ownerId,
                scheduledDatetime = req.scheduledDatetime,
                name = req.name,
                description = req.description,
                inviteCode = inviteCode
            )
        )

        // cria especificação conforme o tipo
        when (req.eventType) {
            EventType.GAMEPLAY -> {
                val details = req.gameplay
                    ?: throw IllegalArgumentException("Gameplay details are required for GAMEPLAY events")

                if (details.gameName.isBlank()) {
                    throw IllegalArgumentException("Gameplay event must have gameName")
                }

                gameplayEventRepository.save(
                    GameplayEventEntity(
                        eventId = event.id,
                        gameId = details.gameId,
                        gameName = details.gameName
                    )
                )
            }

            EventType.SPORT -> {
                val details = req.sport
                    ?: throw IllegalArgumentException("Sport details are required for SPORT events")

                if (details.durationMinutes <= 0) {
                    throw IllegalArgumentException("durationMinutes must be > 0")
                }

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

            else -> {} //Add PARTY futuramente
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
                isInstant = true, // instant = true, group_id = null (check constraint)
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

                if (details.gameName.isBlank()) {
                    throw IllegalArgumentException("Gameplay event must have gameName")
                }

                gameplayEventRepository.save(
                    GameplayEventEntity(
                        eventId = event.id,
                        gameId = details.gameId,
                        gameName = details.gameName
                    )
                )
            }

            EventType.SPORT -> {
                val details = req.sport
                    ?: throw IllegalArgumentException("Sport details are required for SPORT events")

                if (details.durationMinutes <= 0) {
                    throw IllegalArgumentException("durationMinutes must be > 0")
                }

                sportEventRepository.save(
                    SportEventEntity(
                        eventId = event.id,
                        durationMinutes = details.durationMinutes,
                        arena = details.arena,
                        pricePerPlayer = details.pricePerPlayer ?: java.math.BigDecimal.ZERO,
                        maxPlayers = details.maxPlayers,
                        acceptReserve = details.acceptReserve ?: false
                    )
                )
            }

            else -> {} //Add PARTY futuramente
        }

        return buildResponse(event)
    }

    fun listGroupEvents(groupId: UUID, userId: UUID): List<EventResponse> {
        val events = eventRepository.findByGroupIdOrderByScheduledDatetimeAsc(groupId)
        if (events.isEmpty()) return emptyList()

        val eventIds = events.map { it.id }

        // carrega RSVP do usuário logado para esses eventos
        val rsvps = eventRsvpRepository.findByEventIdInAndUserId(eventIds, userId)
        val rsvpByEventId = rsvps.associateBy({ it.eventId }, { it.status }) // status: RsvpStatus

        return events.map { event ->
            buildResponse(
                event = event,
                rsvpStatus = rsvpByEventId[event.id]
            )
        }
    }

    fun getEventById(eventId: UUID): EventResponse {
        val event = eventRepository.findById(eventId)
            .orElseThrow { IllegalArgumentException("Event not found") }

        return buildResponse(event)
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

        return "L-$code" //retorna uma codigo no formato L-A1B2-C3D4
    }

    private
    fun buildResponse(event: EventEntity, rsvpStatus: RsvpStatus? = null): EventResponse {
        val gameplayDetails = if (event.eventType == EventType.GAMEPLAY) {
            gameplayEventRepository.findByEventId(event.id)?.let {
                GameplayEventDetailsResponse(
                    gameId = it.gameId,
                    gameName = it.gameName
                )
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
            gameplay = gameplayDetails,
            sport = sportDetails,
            rsvpStatus = rsvpStatus
        )
    }

}
