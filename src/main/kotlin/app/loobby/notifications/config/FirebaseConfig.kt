package app.loobby.notifications.config

import com.google.auth.oauth2.GoogleCredentials
import com.google.firebase.FirebaseApp
import com.google.firebase.FirebaseOptions
import com.google.firebase.messaging.FirebaseMessaging
import jakarta.annotation.PostConstruct
import org.slf4j.LoggerFactory
import org.springframework.beans.factory.annotation.Value
import org.springframework.context.annotation.Bean
import org.springframework.context.annotation.Configuration
import org.springframework.core.io.ResourceLoader
import java.io.FileInputStream
import java.io.InputStream
import java.nio.file.Paths

/**
 * Inicializa o FirebaseApp default a partir do service account JSON e
 * expõe o [FirebaseMessaging] como bean para injeção no [app.loobby.notifications.service.FcmSender].
 *
 * Quando `loobby.notifications.enabled=false`, nenhuma inicialização é feita e o
 * FcmSender cai em modo "dry run" (apenas loga).
 *
 * O caminho do JSON aceita várias formas:
 *  - Absoluto: `/Users/james/secrets/loobby-fcm.json`
 *  - Relativo ao working dir do processo: `secrets/loobby-fcm.json`
 *  - Com `~`: `~/secrets/loobby-fcm.json`
 *  - Com prefixo Spring: `file:./secrets/loobby-fcm.json`, `classpath:loobby-fcm.json`
 */
@Configuration
class FirebaseConfig(

    @Value("\${loobby.notifications.firebase-credentials-path}")
    private val credentialsPath: String,

    @Value("\${loobby.notifications.enabled}")
    private val enabled: Boolean,

    private val resourceLoader: ResourceLoader
) {
    private val log = LoggerFactory.getLogger(javaClass)

    @PostConstruct
    fun init() {
        if (!enabled) {
            log.warn("Notifications DISABLED — FirebaseApp will NOT be initialized.")
            return
        }

        // Idempotente: só inicializa se não houver FirebaseApp default
        if (FirebaseApp.getApps().isNotEmpty()) {
            log.info("FirebaseApp already initialized; skipping.")
            return
        }

        try {
            openCredentials().use { input ->
                val credentials = GoogleCredentials.fromStream(input)
                val options = FirebaseOptions.builder()
                    .setCredentials(credentials)
                    .build()
                FirebaseApp.initializeApp(options)
            }
            log.info("FirebaseApp initialized from {}", credentialsPath)
        } catch (t: Throwable) {
            log.error(
                "Failed to initialize FirebaseApp from '{}'. " +
                    "Working dir: '{}'. Verifique se o arquivo existe e se o caminho está correto.",
                credentialsPath,
                System.getProperty("user.dir"),
                t
            )
            throw t
        }
    }

    /**
     * Resolve o caminho em múltiplos formatos:
     *  1. Se começar com `~`, expande para o home do usuário.
     *  2. Se tiver prefixo Spring (`classpath:`, `file:`, `http:`...), delega ao ResourceLoader.
     *  3. Caso contrário, tenta como caminho do sistema (absoluto ou relativo ao user.dir).
     *     Relativos implícitos caem no working dir do processo (= diretório do projeto quando
     *     rodando via `./gradlew bootRun`).
     */
    private fun openCredentials(): InputStream {
        val path = credentialsPath.trim()

        if (path.startsWith("~")) {
            val home = System.getProperty("user.home")
            val expanded = home + path.substring(1)
            return FileInputStream(expanded)
        }

        if (path.contains(":") && !path.startsWith("/")) {
            // Prefixos Spring: classpath:, file:, http:, etc.
            val resource = resourceLoader.getResource(path)
            if (!resource.exists()) {
                throw java.io.FileNotFoundException("Resource not found: $path")
            }
            return resource.inputStream
        }

        // Caminho plano: pode ser absoluto ou relativo ao working dir.
        val file = Paths.get(path).toFile()
        if (!file.isAbsolute) {
            val workingDir = System.getProperty("user.dir")
            val resolved = Paths.get(workingDir, path).toFile()
            return FileInputStream(resolved)
        }
        return FileInputStream(file)
    }

    /**
     * Só cria o bean FirebaseMessaging se notifications estiverem habilitadas.
     * Quando desabilitadas, o FcmSender deve ter FirebaseMessaging opcional
     * (tratado com @Autowired(required=false) lá).
     */
    @Bean
    fun firebaseMessaging(): FirebaseMessaging? {
        if (!enabled) return null
        return FirebaseMessaging.getInstance()
    }
}
