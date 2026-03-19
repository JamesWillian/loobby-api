package com.jammes.loobby.events.controller

import com.jammes.loobby.events.dto.CreateEventRequest
import com.jammes.loobby.events.dto.EventResponse
import com.jammes.loobby.events.dto.UpsertRsvpRequest
import com.jammes.loobby.events.dto.EventRsvpResponse
import com.jammes.loobby.events.service.EventRsvpService
import com.jammes.loobby.events.service.EventService
import jakarta.validation.Valid
import org.springframework.security.core.annotation.AuthenticationPrincipal
import org.springframework.security.oauth2.jwt.Jwt
import org.springframework.web.bind.annotation.*
import java.util.UUID

@RestController
@RequestMapping
class EventController(
    private val eventService: EventService,
    private val eventRsvpService: EventRsvpService
) {

    // Evento associado a um grupo
    @PostMapping("/groups/{groupId}/events")
    fun createGroupEvent(
        @AuthenticationPrincipal jwt: Jwt,
        @PathVariable groupId: UUID,
        @Valid @RequestBody req: CreateEventRequest
    ): EventResponse {
        val ownerId = UUID.fromString(jwt.subject)
        return eventService.createGroupEvent(ownerId, groupId, req)
    }

    // Evento instantâneo
    @PostMapping("/events/instant")
    fun createInstantEvent(
        @AuthenticationPrincipal jwt: Jwt,
        @Valid @RequestBody req: CreateEventRequest
    ): EventResponse {
        val ownerId = UUID.fromString(jwt.subject)
        return eventService.createInstantEvent(ownerId, req)
    }

    // Lista eventos de um grupo
    @GetMapping("/groups/{groupId}/events")
    fun listGroupEvents(
        @AuthenticationPrincipal jwt: Jwt,
        @PathVariable groupId: UUID
    ): List<EventResponse> {
        val userId = UUID.fromString(jwt.subject)
        return eventService.listGroupEvents(groupId, userId)
    }

    // Detalhes de um evento — agora inclui o rsvpStatus do usuário autenticado
    @GetMapping("/events/{eventId}")
    fun getEvent(
        @AuthenticationPrincipal jwt: Jwt,
        @PathVariable eventId: UUID
    ): EventResponse {
        val userId = UUID.fromString(jwt.subject)
        return eventService.getEventById(eventId, userId)
    }

    // ----- RSVP -----

    @PutMapping("/events/{eventId}/rsvps")
    fun upsertMyRsvp(
        @AuthenticationPrincipal jwt: Jwt,
        @PathVariable eventId: UUID,
        @Valid @RequestBody req: UpsertRsvpRequest
    ): EventRsvpResponse {
        val userId = UUID.fromString(jwt.subject)
        return eventRsvpService.upsertRsvp(userId, eventId, req)
    }

    @GetMapping("/events/{eventId}/rsvps")
    fun listRsvps(
        @PathVariable eventId: UUID
    ): List<EventRsvpResponse> {
        return eventRsvpService.listRsvps(eventId)
    }

    @GetMapping("/events/{eventId}/rsvps/me")
    fun getMyRsvp(
        @AuthenticationPrincipal jwt: Jwt,
        @PathVariable eventId: UUID
    ): EventRsvpResponse? {
        val userId = UUID.fromString(jwt.subject)
        return eventRsvpService.getMyRsvp(userId, eventId)
    }
}