package app.loobby.users.service

import app.loobby.events.model.RsvpStatus
import app.loobby.events.repo.EventRepository
import app.loobby.events.repo.EventRsvpRepository
import app.loobby.groups.repo.GroupMemberRepository
import app.loobby.groups.repo.GroupRepository
import app.loobby.users.dto.ChangePasswordRequest
import app.loobby.users.dto.DeleteAccountRequest
import com.fasterxml.jackson.annotation.JsonProperty
import app.loobby.users.dto.UpdateUserProfileRequest
import app.loobby.users.dto.UserFeedResponse
import app.loobby.users.dto.UserMeResponse
import app.loobby.users.dto.UserProfileResponse
import app.loobby.users.repo.UserCredentialsRepository
import app.loobby.users.repo.UserFeedRepository
import app.loobby.users.repo.UsersRepository
import org.springframework.http.HttpStatus
import org.springframework.security.crypto.password.PasswordEncoder
import org.springframework.stereotype.Service
import org.springframework.transaction.annotation.Transactional
import org.springframework.web.server.ResponseStatusException
import java.util.UUID
import kotlin.String

@Service
class UsersService(
    private val usersRepository: UsersRepository,
    private val credentialsRepository: UserCredentialsRepository,
    private val userFeedRepository: UserFeedRepository,
    private val passwordEncoder: PasswordEncoder,
    private val groupRepository: GroupRepository,
    private val groupMemberRepository: GroupMemberRepository,
    private val eventRepository: EventRepository,
    private val eventRsvpRepository: EventRsvpRepository,
) {

    fun getMe(userId: UUID): UserMeResponse {
        val user = usersRepository.findById(userId)
            .orElseThrow { IllegalArgumentException("User not found") }

        val credentials = credentialsRepository.findByUserId(userId)

        val roles = credentials?.roles ?: listOf("ANON")

        return UserMeResponse(
            id = user.id,
            username = user.username,
            displayname = user.displayname,
            avatarUrl = user.avatarUrl,
            isAnonymous = credentials == null,
            roles = roles,
            email = credentials?.email,
            emailVerified = credentials?.emailVerified ?: false,
            createdAt = user.createdAt
        )
    }

    @Transactional
    fun updateUserProfile(userId: UUID, req: UpdateUserProfileRequest): UserProfileResponse {
        val user = usersRepository.findById(userId)
            .orElseThrow { IllegalArgumentException("User not found") }

        // Se veio username, valida e altera
        req.username?.let { newUsername ->
            val trimmedUsername = newUsername.trim()
            if (trimmedUsername.isBlank()) {
                throw IllegalArgumentException("Username cannot be blank")
            }

            // garantir que não está usando de outro usuário
            val exists = usersRepository.existsByUsernameIgnoreCase(trimmedUsername)
            if (exists && !trimmedUsername.equals(user.username, ignoreCase = true)) {
                throw IllegalArgumentException("Username is already taken")
            }

            user.username = trimmedUsername
        }

        // displayname pode ser nulo ou vazio → permite limpar
        req.displayname?.let { newDisplayname ->
            user.displayname = newDisplayname.trim().ifBlank { null }
        }

        // avatarUrl também pode ser null → remove avatar
        req.avatarUrl?.let { newAvatarUrl ->
            user.avatarUrl = newAvatarUrl.trim().ifBlank { null }
        }

        val saved = usersRepository.save(user)

        return UserProfileResponse(
            id = saved.id,
            username = saved.username,
            displayname = saved.displayname,
            avatarUrl = saved.avatarUrl,
            createdAt = saved.createdAt
        )
    }

    @Transactional
    fun updateAvatar(userId: UUID, avatarUrl: String): UserProfileResponse {
        val user = usersRepository.findById(userId)
            .orElseThrow { IllegalArgumentException("User not found") }

        user.avatarUrl = avatarUrl

        val saved = usersRepository.save(user)

        return UserProfileResponse(
            id = saved.id,
            username = saved.username,
            displayname = saved.displayname,
            avatarUrl = saved.avatarUrl,
            createdAt = saved.createdAt
        )
    }

    fun getUserFeed(userId: UUID): List<UserFeedResponse> {
//        val user = usersRepository.findById(userId)
//            .orElseThrow { IllegalArgumentException("User not found") }

        val feed = userFeedRepository.findByUserId(userId)

        return feed.map {
            UserFeedResponse(
                id = it.id,
                userId = it.userId,
                name = it.name,
                imageUrl = it.imageUrl,
                entryType = it.entryType,
                isFinished = it.isFinished
            )
        }
    }

    @Transactional
    fun changePassword(userId: UUID, request: ChangePasswordRequest) {
        // Validações
        if (request.newPassword.length < 6) {
            throw ResponseStatusException(HttpStatus.BAD_REQUEST, "A nova senha deve ter pelo menos 6 caracteres.")
        }
        if (request.newPassword != request.confirmPassword) {
            throw ResponseStatusException(HttpStatus.BAD_REQUEST, "As senhas não coincidem.")
        }
        if (request.currentPassword == request.newPassword) {
            throw ResponseStatusException(HttpStatus.BAD_REQUEST, "A nova senha deve ser diferente da atual.")
        }

        val credential = credentialsRepository.findById(userId)
            .orElseThrow { ResponseStatusException(HttpStatus.NOT_FOUND, "Usuário não encontrado.") }

        // Verifica senha atual
        if (!passwordEncoder.matches(request.currentPassword, credential.passwordHash)) {
            throw ResponseStatusException(HttpStatus.UNAUTHORIZED, "Senha atual incorreta.")
        }

        credential.passwordHash = passwordEncoder.encode(request.newPassword).toString()
        credentialsRepository.save(credential)
    }

    @Transactional
    fun deleteAccount(userId: UUID, request: DeleteAccountRequest) {
        // 1. Valida a senha antes de qualquer alteração
        val credential = credentialsRepository.findById(userId)
            .orElseThrow { ResponseStatusException(HttpStatus.NOT_FOUND, "Usuário não encontrado.") }

        if (!passwordEncoder.matches(request.password, credential.passwordHash)) {
            throw ResponseStatusException(HttpStatus.UNAUTHORIZED, "Senha incorreta.")
        }

        // 2. Grupos onde o usuário é dono
        val ownedGroups = groupRepository.findByOwnerId(userId)
        for (group in ownedGroups) {
            val nextOwner = groupMemberRepository.findFirstByGroupIdAndUserIdNot(group.id, userId)
            if (nextOwner != null) {
                // Transfere ownership para qualquer outro membro
                group.ownerId = nextOwner.userId
                groupRepository.save(group)
            } else {
                // Sem outros membros — deleta o grupo
                // CASCADE no banco remove eventos, rsvps, teams e tudo mais associado
                groupRepository.deleteById(group.id)
            }
        }

        // 3. Eventos instantâneos onde o usuário é owner
        // (eventos de grupo já foram cobertos pelo CASCADE acima, se o grupo foi deletado,
        //  ou permanecem com o grupo transferido onde o dono do grupo gerencia)
        val instantEvents = eventRepository.findByOwnerIdAndIsInstantTrue(userId)
        for (event in instantEvents) {
            val nextOwner = eventRsvpRepository
                .findFirstByEventIdAndUserIdNotAndStatusOrderByCreatedAtAsc(
                    event.id, userId, RsvpStatus.YES
                )
            if (nextOwner != null) {
                // Transfere ownership para o confirmado mais antigo
                event.ownerId = nextOwner.userId
                eventRepository.save(event)
            } else {
                // Sem confirmados — deleta o evento
                // CASCADE no banco remove rsvps e detalhes associados
                eventRepository.deleteById(event.id)
            }
        }

        // 4. Zera o campo obs de todos os RSVPs restantes do usuário
        eventRsvpRepository.clearObsByUserId(userId)

        // 5. Deleta as credenciais (remove todos os dados pessoais: email, senha, etc.)
        credentialsRepository.deleteById(userId)

        // 6. Anonimiza o registro em users (mantém o id para integridade referencial)
        val user = usersRepository.findById(userId)
            .orElseThrow { ResponseStatusException(HttpStatus.NOT_FOUND, "Usuário não encontrado.") }
        user.username = "[deleted]"
        user.displayname = null
        user.avatarUrl = null
        usersRepository.save(user)
    }
}
