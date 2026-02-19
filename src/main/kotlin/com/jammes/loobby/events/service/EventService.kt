package com.jammes.loobby.events.service

import com.jammes.loobby.events.dto.CreateEventRequest
import com.jammes.loobby.events.dto.EventResponse
import com.jammes.loobby.events.model.EventEntity
import com.jammes.loobby.events.repo.EventRepository
import com.jammes.loobby.groups.repo.GroupRepository
import org.springframework.stereotype.Service
import java.util.UUID

@Service
class EventService(
    private val eventRepository: EventRepository,
    private val groupRepository: GroupRepository
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

        return event.toResponse()
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

        return event.toResponse()
    }

    fun listGroupEvents(groupId: UUID): List<EventResponse> {
        // garante que o grupo existe
        if (!groupRepository.existsById(groupId)) {
            throw IllegalArgumentException("Group not found")
        }

        return eventRepository.findByGroupIdOrderByScheduledDatetimeAsc(groupId)
            .map { it.toResponse() }
    }

    fun getEventById(eventId: UUID): EventResponse {
        val event = eventRepository.findById(eventId)
            .orElseThrow { IllegalArgumentException("Event not found") }

        return event.toResponse()
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

        return "L-$code" //retorna uma codigo como L-A1B2-C3D4
    }

    private fun EventEntity.toResponse() = EventResponse(
        id = id,
        eventType = eventType,
        groupId = groupId,
        isInstant = isInstant,
        ownerId = ownerId,
        scheduledDatetime = scheduledDatetime,
        name = name,
        description = description,
        inviteCode = inviteCode,
        createdAt = createdAt
    )
}
