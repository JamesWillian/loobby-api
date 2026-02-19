package com.jammes.loobby.events.teams.repo

import com.jammes.loobby.events.teams.model.TeamPlayerEntity
import com.jammes.loobby.events.teams.model.TeamPlayerId
import org.springframework.data.jpa.repository.JpaRepository
import java.util.UUID

interface TeamPlayerRepository : JpaRepository<TeamPlayerEntity, TeamPlayerId> {

    fun findByTeamId(teamId: UUID): List<TeamPlayerEntity>

    fun findByTeamIdIn(teamIds: Collection<UUID>): List<TeamPlayerEntity>

    fun deleteByTeamIdAndUserId(teamId: UUID, userId: UUID)
}
