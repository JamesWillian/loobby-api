package app.loobby.events.controller

import app.loobby.common.web.JsMinifier
import org.springframework.beans.factory.annotation.Value
import org.springframework.core.io.ClassPathResource
import org.springframework.http.MediaType
import org.springframework.web.bind.annotation.GetMapping
import org.springframework.web.bind.annotation.PathVariable
import org.springframework.web.bind.annotation.RequestMapping
import org.springframework.web.bind.annotation.RestController

/**
 * Serve a página estática consumida pelos convidados que clicam no link
 * compartilhado: https://loobby.app/c/{token}
 *
 * Pipeline aplicado uma única vez no primeiro request (lazy + cache):
 *   1. Lê template do classpath
 *   2. Substitui placeholders da config Firebase (apiKey, etc.)
 *   3. Minifica os blocos `<script type="module">` via [JsMinifier]
 *
 * A substituição vem ANTES da minificação para que as chaves reais já estejam
 * embutidas como string literal — o minificador não toca em conteúdo de strings.
 */
@RestController
@RequestMapping("/c")
class PublicWebController(

    @Value("\${loobby.firebase.web-config.api-key:}")
    private val firebaseApiKey: String,

    @Value("\${loobby.firebase.web-config.auth-domain:}")
    private val firebaseAuthDomain: String,

    @Value("\${loobby.firebase.web-config.project-id:}")
    private val firebaseProjectId: String,

    @Value("\${loobby.firebase.web-config.app-id:}")
    private val firebaseAppId: String,

    private val jsMinifier: JsMinifier
) {

    /** HTML final, com placeholders substituídos e JS inline minificado. */
    private val renderedHtml: String by lazy { buildRenderedHtml() }

    @GetMapping("/{token}", produces = [MediaType.TEXT_HTML_VALUE])
    fun rsvpPage(@PathVariable token: String): String = renderedHtml

    // -------------------------------------------------------------------------
    // Internals
    // -------------------------------------------------------------------------

    private fun buildRenderedHtml(): String {
        val template = readTemplate()
        val withConfig = substitutePlaceholders(template)
        return minifyInlineScripts(withConfig)
    }

    private fun readTemplate(): String =
        ClassPathResource("templates/public-rsvp.html")
            .inputStream
            .bufferedReader(Charsets.UTF_8)
            .use { it.readText() }

    private fun substitutePlaceholders(html: String): String =
        html
            .replace("{{FIREBASE_API_KEY}}", firebaseApiKey)
            .replace("{{FIREBASE_AUTH_DOMAIN}}", firebaseAuthDomain)
            .replace("{{FIREBASE_PROJECT_ID}}", firebaseProjectId)
            .replace("{{FIREBASE_APP_ID}}", firebaseAppId)

    /**
     * Encontra cada bloco `<script type="module">…</script>` e roda o conteúdo
     * pelo [JsMinifier]. Tags com outros tipos (ou sem `type=module`) não são
     * tocadas — evita minificar acidentalmente analytics/scripts externos.
     */
    private fun minifyInlineScripts(html: String): String =
        INLINE_MODULE_SCRIPT_REGEX.replace(html) { match ->
            val openTag = match.groupValues[1]
            val js = match.groupValues[2]
            val closeTag = match.groupValues[3]
            val minified = jsMinifier.minify(js)
            "$openTag$minified$closeTag"
        }

    private companion object {
        /**
         * Captura abertura, corpo e fechamento de `<script type="module">`.
         * `[\s\S]` em vez de `.` para casar com quebras de linha.
         */
        private val INLINE_MODULE_SCRIPT_REGEX = Regex(
            """(<script\s+type="module"\s*>)([\s\S]*?)(</script>)""",
            RegexOption.IGNORE_CASE
        )
    }
}
