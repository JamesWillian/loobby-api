package app.loobby.notifications.service

import app.loobby.notifications.dto.RegisterDeviceRequest
import app.loobby.notifications.model.DeviceTokenEntity
import app.loobby.notifications.repo.DeviceTokenRepository
import org.springframework.stereotype.Service
import org.springframework.transaction.annotation.Transactional
import java.util.UUID

@Service
class DeviceTokenService(
    private val repository: DeviceTokenRepository
) {
    /**
     * Upsert do token: se já existe, atualiza userId/platform (os campos de timestamp
     * são geridos automaticamente pelo Hibernate). Cobre o caso de um mesmo device
     * trocar de usuário (logout + login de outra conta).
     */
    @Transactional
    fun register(userId: UUID, request: RegisterDeviceRequest) {
        val existing = repository.findById(request.token).orElse(null)
        if (existing != null) {
            existing.userId = userId
            existing.platform = request.platform
            repository.save(existing)
        } else {
            repository.save(
                DeviceTokenEntity(
                    token = request.token,
                    userId = userId,
                    platform = request.platform
                )
            )
        }
    }

    /**
     * Chamado no logout para o token não continuar recebendo pushes da conta antiga.
     */
    @Transactional
    fun unregister(token: String) {
        repository.deleteById(token)
    }
}
