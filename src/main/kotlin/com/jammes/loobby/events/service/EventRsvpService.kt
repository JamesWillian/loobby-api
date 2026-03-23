package com.jammes.loobby.events.service

import com.jammes.loobby.events.dto.EventRsvpResponse
import com.jammes.loobby.events.dto.UpsertRsvpRequest
import com.jammes.loobby.events.model.EventRsvpEntity
import com.jammes.loobby.events.repo.EventRepository
import com.jammes.loobby.events.repo.EventRsvpRepository
import com.jammes.loobby.groups.repo.GroupMemberRepository
import com.jammes.loobby.users.repo.UsersRepository
import org.springframework.stereotype.Service
import org.springframework.transaction.annotation.Transactional
import java.util.UUID

@Service
class EventRsvpService(
    private val eventRepository: EventRepository,
    private val eventRsvpRepository: EventRsvpRepository,
    private val usersRepository: UsersRepository,
    private val groupMemberRepository: GroupMemberRepository
) {

    @Transactional
    fun upsertRsvp(userId: UUID, eventId: UUID, req: UpsertRsvpRequest): EventRsvpResponse {
        val event = eventRepository.findById(eventId)
            .orElseThrow { IllegalArgumentException("Event not found") }

        // Regra: se evento é de grupo, só membro pode dar RSVP
        event.groupId?.let { groupId ->
            val isMember = groupMemberRepository.existsByGroupIdAndUserId(groupId, userId)
            if (!isMember && event.ownerId != userId) {
                throw IllegalStateException("User is not member of this group")
            }
        }

        val existing = eventRsvpRepository.findByEventIdAndUserId(eventId, userId)

        val saved = if (existing == null) {
            eventRsvpRepository.save(
                EventRsvpEntity(
                    eventId = eventId,
                    userId = userId,
                    status = req.status,
                    isPaid = req.isPaid ?: false,
                    obs = req.obs
                )
            )
        } else {
            existing.status = req.status
            if (req.isPaid != null) {
                existing.isPaid = req.isPaid
            }
            existing.obs = req.obs
            eventRsvpRepository.save(existing)
        }

        val user = usersRepository.findById(userId)
            .orElseThrow { IllegalArgumentException("User not found") }

        return EventRsvpResponse(
            eventId = saved.eventId,
            userId = saved.userId,
            status = saved.status,
            isPaid = saved.isPaid,
            obs = saved.obs,
            createdAt = saved.createdAt,
            username = user.username,
            displayname = user.displayname,
            avatarUrl = user.avatarUrl,
            isOwner = (user.id == event.ownerId)
        )
    }

    fun listRsvps(eventId: UUID): List<EventRsvpResponse> {
        val event = eventRepository.findById(eventId)
            .orElseThrow { IllegalArgumentException("Event not found") }

        val rsvps = eventRsvpRepository.findByEventIdOrderByCreatedAtAsc(eventId)
        if (rsvps.isEmpty()) return emptyList()

        val userIds = rsvps.map { it.userId }.toSet()
        val users = usersRepository.findAllById(userIds)
        val usersById = users.associateBy { it.id }

        return rsvps.mapNotNull { rsvp ->
            val user = usersById[rsvp.userId] ?: return@mapNotNull null

            EventRsvpResponse(
                eventId = rsvp.eventId,
                userId = rsvp.userId,
                status = rsvp.status,
                isPaid = rsvp.isPaid,
                obs = rsvp.obs,
                createdAt = rsvp.createdAt,
                username = user.username,
                displayname = user.displayname,
                avatarUrl = user.avatarUrl,
                isOwner = (user.id == event.ownerId)
            )
        }
    }

    fun getMyRsvp(userId: UUID, eventId: UUID): EventRsvpResponse? {
        val event = eventRepository.findById(eventId)
            .orElseThrow { IllegalArgumentException("Event not found") }

        val rsvp = eventRsvpRepository.findByEventIdAndUserId(eventId, userId)
            ?: return null

        val user = usersRepository.findById(userId)
            .orElseThrow { IllegalArgumentException("User not found") }

        return EventRsvpResponse(
            eventId = rsvp.eventId,
            userId = rsvp.userId,
            status = rsvp.status,
            isPaid = rsvp.isPaid,
            obs = rsvp.obs,
            createdAt = rsvp.createdAt,
            username = user.username,
            displayname = user.displayname,
            avatarUrl = user.avatarUrl,
            isOwner = (user.id == event.ownerId)
        )
    }
}
