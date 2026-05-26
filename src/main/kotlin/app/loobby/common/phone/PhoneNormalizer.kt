package app.loobby.common.phone

import com.google.i18n.phonenumbers.NumberParseException
import com.google.i18n.phonenumbers.PhoneNumberUtil

/**
 * Normaliza telefones para o formato E.164 (ex.: "+5511999998888").
 *
 * Usado em qualquer fluxo onde o telefone é identificador de usuário —
 * principalmente o RSVP por link público e a verificação por WhatsApp.
 *
 * Lança [IllegalArgumentException] quando o número é inválido. Os services
 * capturam isso e devolvem o erro adequado ao chamador.
 */
object PhoneNormalizer {

    private val phoneUtil: PhoneNumberUtil = PhoneNumberUtil.getInstance()

    /**
     * Converte um telefone digitado pelo usuário para E.164.
     *
     * @param raw número como digitado ("11 99999-8888", "(11) 99999-8888",
     *            "+55 11 99999-8888", etc.).
     * @param defaultRegion código ISO-3166 do país assumido quando o número
     *                      não vier com DDI explícito. Default "BR".
     * @return número no formato E.164 (ex.: "+5511999998888").
     * @throws IllegalArgumentException se o número for vazio, em formato
     *         inválido ou semanticamente inválido para a região.
     */
    fun toE164(raw: String, defaultRegion: String = "BR"): String {
        require(raw.isNotBlank()) { "phone is required" }

        val parsed = try {
            phoneUtil.parse(raw.trim(), defaultRegion)
        } catch (e: NumberParseException) {
            throw IllegalArgumentException("invalid phone format: ${e.errorType.name}", e)
        }

        require(phoneUtil.isValidNumber(parsed)) { "invalid phone number" }

        return phoneUtil.format(parsed, PhoneNumberUtil.PhoneNumberFormat.E164)
    }
}
