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
import java.io.FileInputStream

/**
 * Inicializa o FirebaseApp default a partir do service account JSON e
 * expõe o [FirebaseMessaging] como bean para injeção no [app.loobby.notifications.service.FcmSender].
 *
 * Quando `loobby.notifications.enabled=false`, nenhuma inicialização é feita e o
 * FcmSender cai em modo "dry run" (apenas loga).
 */
@Configuration
class FirebaseConfig(

    @Value("\${loobby.notifications.firebase-credentials-path}")
    private val credentialsPath: String,

    @Value("\${loobby.notifications.enabled}")
    private val enabled: Boolean
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
            val credentials = FileInputStream(credentialsPath).use {
                GoogleCredentials.fromStream(it)
            }
            val options = FirebaseOptions.builder()
                .setCredentials(credentials)
                .build()
            FirebaseApp.initializeApp(options)
            log.info("FirebaseApp initialized from {}", credentialsPath)
        } catch (t: Throwable) {
            log.error("Failed to initialize FirebaseApp from $credentialsPath", t)
            throw t
        }
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
