package app.loobby.groups.dto

import java.time.Instant
import java.util.UUID

data class GroupResponse(
    val id: UUID,
    val name: String,
    val inviteCode: String,
    val imageUrl: String?,
    val ownerId: UUID,
    val createdAt: Instant?
)
