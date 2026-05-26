package app.loobby.users.service

import app.loobby.events.repo.EventRsvpRepository
import app.loobby.users.repo.UsersRepository
import org.slf4j.LoggerFactory
import org.springframework.stereotype.Service
import org.springframework.transaction.annotation.Transactional
import java.util.UUID

/**
 * Resultado da operação de vincular telefone ao usuário corrente, mesclando
 * uma eventual conta lite criada anteriormente.
 *
 * Cada outcome corresponde a um caminho semântico distinto que o chamador
 * (AuthService, GoogleAuthService) precisa tratar diferente:
 *   - [MERGED]: havia uma conta lite com esse telefone — RSVPs foram movidas
 *     para o usuário atual e a conta lite foi descartada.
 *   - [PHONE_ATTACHED]: telefone era novo ou já era do próprio usuário; nada
 *     a mesclar, só atualizamos o cadastro.
 *   - [REJECTED_FULL_ACCOUNT]: o telefone pertence a outra conta full — o
 *     chamador deve abortar o registro com 409 e instruir o usuário a fazer
 *     login na conta existente.
 */
enum class PhoneAttachOutcome {
    MERGED,
    PHONE_ATTACHED,
    REJECTED_FULL_ACCOUNT
}

/**
 * Vincula um telefone (já verificado por OTP) ao usuário atual e, se houver
 * uma conta lite com o mesmo telefone, mescla seus RSVPs no usuário atual.
 *
 * Aderência a SOLID:
 *  - SRP: a única responsabilidade desta classe é executar essa transação
 *    de "anexar telefone + eventual merge". Verificação de OTP, normalização
 *    e regras de autenticação ficam em outras classes.
 *  - DIP: depende apenas de [UsersRepository] e [EventRsvpRepository]
 *    (interfaces do Spring Data), nada de provedor de auth ou HTTP.
 */
@Service
class UserMergeService(
    private val usersRepository: UsersRepository,
    private val rsvpRepository: EventRsvpRepository
) {

    private val log = LoggerFactory.getLogger(javaClass)

    /**
     * Anexa o telefone ao usuário corrente; caso outro usuário (lite,
     * `authProvider = 0`) já possua esse telefone, mescla os dados desse
     * lite no usuário corrente e descarta o lite.
     *
     * Pré-condição: o telefone JÁ deve ter sido verificado por OTP antes
     * da chamada — esta service não verifica posse.
     *
     * @return o desfecho aplicado ([PhoneAttachOutcome]).
     */
    @Transactional
    fun attachPhoneAndMergeLite(currentUserId: UUID, verifiedPhoneE164: String): PhoneAttachOutcome {
        val current = usersRepository.findById(currentUserId)
            .orElseThrow { IllegalArgumentException("Current user not found: $currentUserId") }

        val existing = usersRepository.findByPhoneE164(verifiedPhoneE164)

        // Caso 1: nenhum usuário com esse telefone, ou é o próprio current.
        if (existing == null || existing.id == currentUserId) {
            if (current.phoneE164 != verifiedPhoneE164) {
                current.phoneE164 = verifiedPhoneE164
                usersRepository.save(current)
            }
            return PhoneAttachOutcome.PHONE_ATTACHED
        }

        // Caso 2: telefone pertence a uma conta full — bloquear.
        if (existing.authProvider != AUTH_PROVIDER_ANON) {
            log.warn(
                "Phone attach rejected: phone {} belongs to full account {} (authProvider={}); requester={}",
                verifiedPhoneE164, existing.id, existing.authProvider, currentUserId
            )
            return PhoneAttachOutcome.REJECTED_FULL_ACCOUNT
        }

        // Caso 3: lite. Mesclar.
        mergeLiteInto(liteUserId = existing.id, currentUserId = currentUserId)

        current.phoneE164 = verifiedPhoneE164
        usersRepository.save(current)

        return PhoneAttachOutcome.MERGED
    }

    // -------------------------------------------------------------------------
    // Internals
    // -------------------------------------------------------------------------

    /**
     * Transfere todas as RSVPs do usuário lite para o atual e remove o lite.
     *
     * Estratégia em duas passadas para respeitar a PK composta de event_rsvps:
     *  1. Move RSVPs do lite para o atual onde não há conflito (atual ainda não
     *     tem RSVP no mesmo evento).
     *  2. Apaga as RSVPs do lite que não foram movidas (eram conflitos —
     *     ficamos com a RSVP do usuário atual, que é a mais "real").
     *  3. Apaga o registro do lite (cascades em outras tabelas, se houver).
     */
    private fun mergeLiteInto(liteUserId: UUID, currentUserId: UUID) {
        val reassigned = rsvpRepository.reassignUserIfNoConflict(
            fromUserId = liteUserId,
            toUserId = currentUserId
        )
        val droppedConflicts = rsvpRepository.deleteByUserId(liteUserId)

        usersRepository.deleteById(liteUserId)

        log.info(
            "Merged lite user {} into {}: reassigned={} RSVPs, droppedConflicts={}",
            liteUserId, currentUserId, reassigned, droppedConflicts
        )
    }

    private companion object {
        /** Valor de [app.loobby.users.model.UserEntity.authProvider] para usuários anônimos/lite. */
        const val AUTH_PROVIDER_ANON = 0
    }
}
