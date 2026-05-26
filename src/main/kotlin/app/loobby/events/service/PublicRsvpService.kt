package app.loobby.events.service

import app.loobby.common.phone.PhoneNormalizer
import app.loobby.common.phone.PhoneVerificationException
import app.loobby.common.phone.PhoneVerifier
import app.loobby.events.dto.ConfirmConfirmationResponse
import app.loobby.events.dto.PublicEventResponse
import app.loobby.events.dto.StartConfirmationResponse
import app.loobby.events.dto.StartConfirmationStatus
import app.loobby.events.model.EventLinkTokenEntity
import app.loobby.events.model.EventRsvpEntity
import app.loobby.events.model.EventRsvpPendingEntity
import app.loobby.events.model.RsvpSource
import app.loobby.events.model.RsvpStatus
import app.loobby.events.model.RsvpVerificationLevel
import app.loobby.events.repo.EventRepository
import app.loobby.events.repo.EventRsvpPendingRepository
import app.loobby.events.repo.EventRsvpRepository
import app.loobby.users.model.UserEntity
import app.loobby.users.repo.UsersRepository
import org.slf4j.LoggerFactory
import org.springframework.beans.factory.annotation.Value
import org.springframework.dao.DataIntegrityViolationException
import org.springframework.http.HttpStatus
import org.springframework.stereotype.Service
import org.springframework.transaction.annotation.Transactional
import org.springframework.web.server.ResponseStatusException
import java.time.Duration
import java.time.Instant
import java.util.UUID

/**
 * Orquestra o fluxo de RSVP via link público (loobby.app/c/{token}).
 *
 * Composto por duas etapas:
 *   1. [startConfirmation]  → cria/atualiza event_rsvp_pending, devolve phone
 *                              normalizado para o cliente disparar Firebase Phone Auth
 *   2. [confirmConfirmation] → valida o ID token do Firebase, promove o pending
 *                              para event_rsvps com verification_level=PHONE_VERIFIED
 *
 * Aderência a SOLID:
 *   - SRP: este service só orquestra o fluxo. Normalização ([PhoneNormalizer]),
 *     verificação de posse ([PhoneVerifier]) e resolução do link ([LinkTokenService])
 *     vivem cada um na sua casa.
 *   - DIP: depende de [PhoneVerifier] (interface), não da implementação Firebase
 *     concretamente — fácil trocar de provedor ou mockar em teste.
 */
@Service
class PublicRsvpService(
    private val linkTokenService: LinkTokenService,
    private val eventRepository: EventRepository,
    private val usersRepository: UsersRepository,
    private val rsvpRepository: EventRsvpRepository,
    private val pendingRepository: EventRsvpPendingRepository,
    private val phoneVerifier: PhoneVerifier,

    @Value("\${loobby.public.pending-rsvp-ttl-hours:24}")
    private val pendingRsvpTtlHours: Long
) {

    private val log = LoggerFactory.getLogger(javaClass)

    // -------------------------------------------------------------------------
    // Read API
    // -------------------------------------------------------------------------

    /** Dados públicos do evento, mostrados na tela inicial do link. */
    @Transactional(readOnly = true)
    fun getPublicEvent(rawToken: String): PublicEventResponse {
        val token = resolveTokenOrThrow(rawToken)

        val event = eventRepository.findById(token.eventId)
            .orElseThrow { ResponseStatusException(HttpStatus.NOT_FOUND, "event not found") }

        val confirmedCount = rsvpRepository
            .countByEventIdAndStatus(event.id, RsvpStatus.YES)
            .toInt()
        val owner = usersRepository.findById(event.ownerId).orElse(null)

        return PublicEventResponse(
            id = event.id,
            eventType = event.eventType,
            name = event.name,
            description = event.description,
            scheduledDatetime = event.scheduledDatetime,
            confirmedCount = confirmedCount,
            ownerDisplayname = owner?.displayname
        )
    }

    // -------------------------------------------------------------------------
    // Write API — etapa 1 (start)
    // -------------------------------------------------------------------------

    /**
     * Etapa 1: normaliza o telefone, verifica se já há RSVP confirmada para essa
     * combinação evento+telefone e, se não houver, cria/atualiza o pending.
     *
     * Não envia OTP — quem dispara o SMS é o Firebase via SDK no client.
     */
    @Transactional
    fun startConfirmation(rawToken: String, name: String, rawPhone: String): StartConfirmationResponse {
        val token = resolveTokenOrThrow(rawToken)

        val phoneE164 = normalizePhoneOrThrow(rawPhone)

        // Idempotência: se esse telefone já está confirmado neste evento,
        // pula o OTP e devolve resultado direto.
        if (isAlreadyConfirmed(token.eventId, phoneE164)) {
            return StartConfirmationResponse(
                status = StartConfirmationStatus.ALREADY_CONFIRMED,
                phone = phoneE164,
                pendingId = null
            )
        }

        val pending = upsertPending(token.eventId, phoneE164, name)

        return StartConfirmationResponse(
            status = StartConfirmationStatus.NEED_OTP,
            phone = phoneE164,
            pendingId = pending.id
        )
    }

    // -------------------------------------------------------------------------
    // Write API — etapa 2 (confirm)
    // -------------------------------------------------------------------------

    /**
     * Etapa 2: valida o Firebase ID token (prova de posse do telefone), bate
     * com o pending e promove para event_rsvps com verification_level=PHONE_VERIFIED.
     */
    @Transactional
    fun confirmConfirmation(
        rawToken: String,
        pendingId: UUID,
        firebaseIdToken: String
    ): ConfirmConfirmationResponse {
        val token = resolveTokenOrThrow(rawToken)
        val verifiedPhone = verifyPhoneOrThrow(firebaseIdToken)

        val pending = pendingRepository.findById(pendingId)
            .orElseThrow {
                ResponseStatusException(HttpStatus.NOT_FOUND, "pending not found or expired")
            }

        validatePendingMatches(pending, token, verifiedPhone)

        val user = findOrCreateLiteUser(verifiedPhone, pending.displayName)

        // Idempotência explícita: se já há RSVP para esse evento/usuário,
        // devolve sucesso sem tentar reinserir.
        val existingRsvp = rsvpRepository.findByEventIdAndUserId(pending.eventId, user.id)
        if (existingRsvp == null) {
            // Caminho normal: insere a RSVP. Capturamos
            // DataIntegrityViolationException como rede de segurança contra
            // races (duas chamadas /confirm chegando juntas) ou violação de
            // check constraint do banco — nesse último caso, propagamos um
            // 500 com mensagem clara em vez do 409 genérico do Spring.
            try {
                rsvpRepository.save(
                    EventRsvpEntity(
                        eventId = pending.eventId,
                        userId = user.id,
                        status = RsvpStatus.YES,
                        isPaid = false,
                        obs = null,
                        source = RsvpSource.WEB_LINK,
                        verificationLevel = RsvpVerificationLevel.PHONE_VERIFIED
                    )
                )
            } catch (e: DataIntegrityViolationException) {
                // Recheca se foi race condition (RSVP inserida em paralelo).
                val recheck = rsvpRepository.findByEventIdAndUserId(pending.eventId, user.id)
                if (recheck == null) {
                    log.error(
                        "RSVP insert failed and not present after recheck — " +
                            "verifique check constraints em event_rsvps (source/verification_level) " +
                            "e estado do banco. event={} user={}",
                        pending.eventId, user.id, e
                    )
                    throw ResponseStatusException(
                        HttpStatus.INTERNAL_SERVER_ERROR,
                        "não foi possível registrar a confirmação — contate o suporte"
                    )
                }
                log.info("Race detectada na RSVP — tratando como idempotente. event={} user={}",
                    pending.eventId, user.id)
            }
        }

        pendingRepository.delete(pending)

        return ConfirmConfirmationResponse(
            userId = user.id,
            name = user.displayname ?: pending.displayName,
            phone = verifiedPhone
        )
    }

    // -------------------------------------------------------------------------
    // Internals
    // -------------------------------------------------------------------------

    private fun resolveTokenOrThrow(rawToken: String): EventLinkTokenEntity =
        linkTokenService.resolve(rawToken)
            ?: throw ResponseStatusException(HttpStatus.NOT_FOUND, "invalid or expired link")

    private fun normalizePhoneOrThrow(rawPhone: String): String = try {
        PhoneNormalizer.toE164(rawPhone)
    } catch (e: IllegalArgumentException) {
        throw ResponseStatusException(HttpStatus.BAD_REQUEST, e.message)
    }

    private fun verifyPhoneOrThrow(firebaseIdToken: String): String = try {
        phoneVerifier.verify(firebaseIdToken)
    } catch (e: PhoneVerificationException) {
        throw ResponseStatusException(HttpStatus.UNAUTHORIZED, e.message)
    }

    private fun isAlreadyConfirmed(eventId: UUID, phoneE164: String): Boolean {
        val user = usersRepository.findByPhoneE164(phoneE164) ?: return false
        return rsvpRepository.findByEventIdAndUserId(eventId, user.id) != null
    }

    /**
     * Cria um novo pending ou renova um existente (mesmo evento+telefone).
     * Atualiza display_name caso o usuário tenha corrigido a digitação,
     * incrementa otp_attempts (sinal pro rate limiter) e estende expires_at.
     */
    private fun upsertPending(
        eventId: UUID,
        phoneE164: String,
        displayName: String
    ): EventRsvpPendingEntity {
        val now = Instant.now()
        val expires = now.plus(Duration.ofHours(pendingRsvpTtlHours))

        val existing = pendingRepository.findByEventIdAndPhoneE164(eventId, phoneE164)
        if (existing != null) {
            existing.displayName = displayName
            existing.otpAttempts += 1
            existing.otpSentAt = now
            existing.expiresAt = expires
            return pendingRepository.save(existing)
        }

        return pendingRepository.save(
            EventRsvpPendingEntity(
                eventId = eventId,
                phoneE164 = phoneE164,
                displayName = displayName,
                otpSentAt = now,
                otpAttempts = 1,
                expiresAt = expires
            )
        )
    }

    private fun validatePendingMatches(
        pending: EventRsvpPendingEntity,
        token: EventLinkTokenEntity,
        verifiedPhone: String
    ) {
        if (pending.eventId != token.eventId) {
            throw ResponseStatusException(HttpStatus.BAD_REQUEST, "pending does not belong to this event")
        }
        if (pending.phoneE164 != verifiedPhone) {
            throw ResponseStatusException(
                HttpStatus.BAD_REQUEST,
                "verified phone does not match the one submitted on start"
            )
        }
        if (pending.expiresAt.isBefore(Instant.now())) {
            throw ResponseStatusException(HttpStatus.GONE, "pending expired — comece de novo")
        }
    }

    /**
     * Localiza o usuário existente pelo telefone (E.164 já é único) ou cria
     * um novo usuário "lite": sem credenciais, sem login no app, identificado
     * apenas pelo número.
     *
     * Quando esse usuário eventualmente fizer signup no app com o mesmo
     * telefone, o [app.loobby.users.service.UserMergeService] mescla as
     * RSVPs e descarta o lite.
     *
     * Captura [DataIntegrityViolationException] como rede de segurança:
     *  - Race condition entre duas chamadas /confirm criando o mesmo lite
     *  - Colisão de `username` ou `phone_e164` por estado residual do banco
     * Em qualquer um desses casos, refazemos o lookup. Se ainda assim não
     * achar, logamos e propagamos um erro claro em vez do 409 genérico.
     */
    private fun findOrCreateLiteUser(phoneE164: String, displayName: String): UserEntity {
        usersRepository.findByPhoneE164(phoneE164)?.let { return it }

        return try {
            usersRepository.save(
                UserEntity(
                    id = UUID.randomUUID(),
                    username = generateLiteUsername(phoneE164),
                    displayname = displayName,
                    avatarUrl = null,
                    authProvider = 0,
                    phoneE164 = phoneE164
                )
            )
        } catch (e: DataIntegrityViolationException) {
            usersRepository.findByPhoneE164(phoneE164)?.let { return it }
            log.error(
                "Falha ao criar lite user e nada encontrado no recheck. " +
                    "Possíveis causas: colisão de username='{}' ou estado inconsistente. phone={}",
                generateLiteUsername(phoneE164), phoneE164, e
            )
            throw ResponseStatusException(
                HttpStatus.INTERNAL_SERVER_ERROR,
                "não foi possível registrar — telefone em conflito no servidor"
            )
        }
    }

    /**
     * Username determinístico para usuários lite. Como phone_e164 é UNIQUE
     * em users, esse valor também é único — sem risco de colisão dentro
     * dos usuários lite.
     */
    private fun generateLiteUsername(phoneE164: String): String =
        "lite_${phoneE164.removePrefix("+")}"
}
