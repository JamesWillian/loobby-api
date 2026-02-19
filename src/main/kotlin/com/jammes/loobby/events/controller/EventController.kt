package com.jammes.loobby.events.controller

import com.jammes.loobby.events.dto.CreateEventRequest
import com.jammes.loobby.events.dto.EventResponse
import com.jammes.loobby.events.service.EventService
import jakarta.validation.Valid
import org.springframework.security.core.annotation.AuthenticationPrincipal
import org.springframework.security.oauth2.jwt.Jwt
import org.springframework.web.bind.annotation.*
import java.util.UUID

@RestController
@RequestMapping
class EventController(
    private val eventService: EventService
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

    // Evento instantâneo (sem grupo)
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
        @PathVariable groupId: UUID
    ): List<EventResponse> {
        return eventService.listGroupEvents(groupId)
    }

    // Detalhes de um evento
    @GetMapping("/events/{eventId}")
    fun getEvent(
        @PathVariable eventId: UUID
    ): EventResponse {
        return eventService.getEventById(eventId)
    }
}
