package app.loobby.notifications.repo

import app.loobby.notifications.model.DeviceTokenEntity
import org.springframework.data.jpa.repository.JpaRepository
import org.springframework.data.jpa.repository.Modifying
import org.springframework.data.jpa.repository.Query
import org.springframework.data.repository.query.Param
import java.util.UUID

interface DeviceTokenRepository : JpaRepository<DeviceTokenEntity, String> {

    fun findAllByUserId(userId: UUID): List<DeviceTokenEntity>

    fun findAllByUserIdIn(userIds: Collection<UUID>): List<DeviceTokenEntity>

    @Modifying
    @Query("DELETE FROM DeviceTokenEntity d WHERE d.token IN :tokens")
    fun deleteAllByTokenIn(@Param("tokens") tokens: Collection<String>)
}
