package app.loobby.legal.controller

import org.springframework.core.io.ClassPathResource
import org.springframework.http.CacheControl
import org.springframework.http.MediaType
import org.springframework.http.ResponseEntity
import org.springframework.web.bind.annotation.GetMapping
import org.springframework.web.bind.annotation.RequestMapping
import org.springframework.web.bind.annotation.RestController
import java.util.concurrent.TimeUnit

/**
 * Serve documentos legais públicos (Política de Privacidade, Termos, etc.)
 * em HTML, sem exigir autenticação.
 *
 * As rotas precisam estar whitelistadas em [SecurityConfig] sob /legal/...
 */
@RestController
@RequestMapping("/legal")
class LegalController {

    private val privacyHtml: String by lazy { readResource("legal/privacy.html") }
    private val accountDeletionHtml: String by lazy { readResource("legal/account-deletion.html") }
    private val childSafetyHtml: String by lazy { readResource("legal/child-safety.html") }

    @GetMapping("/privacy", produces = [MediaType.TEXT_HTML_VALUE])
    fun privacy(): ResponseEntity<String> = htmlResponse(privacyHtml)

    @GetMapping("/account-deletion", produces = [MediaType.TEXT_HTML_VALUE])
    fun accountDeletion(): ResponseEntity<String> = htmlResponse(accountDeletionHtml)

    @GetMapping("/child-safety", produces = [MediaType.TEXT_HTML_VALUE])
    fun childSafety(): ResponseEntity<String> = htmlResponse(childSafetyHtml)

    private fun htmlResponse(body: String): ResponseEntity<String> =
        ResponseEntity.ok()
            .contentType(MediaType.parseMediaType("text/html;charset=UTF-8"))
            .cacheControl(CacheControl.maxAge(1, TimeUnit.HOURS).cachePublic())
            .body(body)

    private fun readResource(path: String): String =
        ClassPathResource(path).inputStream.use { it.readBytes().toString(Charsets.UTF_8) }
}
