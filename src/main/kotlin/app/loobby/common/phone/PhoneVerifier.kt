package app.loobby.common.phone

/**
 * Verifica a posse de um número de telefone a partir de um token de OTP.
 *
 * Abstração de provedor (Dependency Inversion + Open/Closed): hoje a única
 * implementação é Firebase Phone Auth ([FirebasePhoneVerifier]); amanhã poderia
 * ser WhatsApp Business, Twilio, Auth0, etc. — basta criar outra implementação.
 *
 * Tudo o que os services precisam saber é "dado este token, qual telefone está
 * provado". Detalhes de OTP, SMS, JWT, claims, etc., ficam encapsulados.
 */
interface PhoneVerifier {

    /**
     * Valida o token de verificação e devolve o telefone associado no formato
     * E.164 (ex.: "+5511999998888").
     *
     * @param verificationToken token opaco gerado pelo provedor de OTP no client
     * @return número de telefone verificado, em E.164
     * @throws PhoneVerificationException quando o token é inválido, expirado ou
     *         não contém claim de telefone.
     */
    fun verify(verificationToken: String): String
}

/**
 * Falha na verificação de posse de telefone. Os controllers convertem isso
 * em HTTP 401 (token inválido) ou 400 (token sem claim de telefone).
 */
class PhoneVerificationException(
    message: String,
    cause: Throwable? = null
) : RuntimeException(message, cause)
