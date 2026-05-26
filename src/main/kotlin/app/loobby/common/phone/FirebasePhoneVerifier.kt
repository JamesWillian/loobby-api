package app.loobby.common.phone

import com.google.firebase.auth.FirebaseAuth
import com.google.firebase.auth.FirebaseAuthException
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.stereotype.Component

/**
 * Implementação de [PhoneVerifier] usando Firebase Phone Authentication.
 *
 * O fluxo é:
 *   1. Cliente faz signInWithPhoneNumber() no Firebase JS SDK / Android SDK
 *   2. Firebase envia SMS, valida código, devolve um ID token JWT
 *   3. Cliente envia o ID token para o backend (em /rsvps/confirm ou /auth/register)
 *   4. Backend chama [verify], que valida assinatura e extrai o claim phone_number
 *
 * O [FirebaseAuth] é opcional para suportar o caso em que `loobby.notifications.enabled=false`
 * desliga toda a inicialização do FirebaseApp — nesse cenário, este componente
 * existe mas qualquer chamada falha com [PhoneVerificationException].
 */
@Component
class FirebasePhoneVerifier(
    @Autowired(required = false)
    private val firebaseAuth: FirebaseAuth?
) : PhoneVerifier {

    override fun verify(verificationToken: String): String {
        val auth = firebaseAuth
            ?: throw PhoneVerificationException("Firebase Auth not configured on this environment")

        if (verificationToken.isBlank()) {
            throw PhoneVerificationException("verification token is required")
        }

        val decoded = try {
            // checkRevoked=true força lookup no Firebase para detectar tokens
            // revogados desde a emissão. É uma chamada de rede extra, mas é
            // barata e fecha uma porta de segurança.
            auth.verifyIdToken(verificationToken, true)
        } catch (e: FirebaseAuthException) {
            throw PhoneVerificationException("invalid or expired verification token", e)
        }

        val phone = decoded.claims["phone_number"] as? String
        if (phone.isNullOrBlank()) {
            throw PhoneVerificationException(
                "verification token has no phone_number claim — fluxo de Phone Auth não foi concluído"
            )
        }

        return phone
    }
}
