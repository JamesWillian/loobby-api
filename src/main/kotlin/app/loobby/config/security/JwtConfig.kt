package app.loobby.config.security

import com.nimbusds.jose.JWSAlgorithm
import com.nimbusds.jose.jwk.JWKSet
import com.nimbusds.jose.jwk.KeyUse
import com.nimbusds.jose.jwk.OctetSequenceKey
import com.nimbusds.jose.jwk.source.ImmutableJWKSet
import javax.crypto.SecretKey
import javax.crypto.spec.SecretKeySpec
import org.springframework.context.annotation.Bean
import org.springframework.context.annotation.Configuration
import org.springframework.security.oauth2.jwt.JwtDecoder
import org.springframework.security.oauth2.jwt.JwtEncoder
import org.springframework.security.oauth2.jwt.NimbusJwtDecoder
import org.springframework.security.oauth2.jwt.NimbusJwtEncoder
import com.nimbusds.jose.proc.SecurityContext
import org.springframework.boot.context.properties.ConfigurationProperties

@Configuration
@ConfigurationProperties(prefix = "security.jwt")
class JwtConfig {

    lateinit var secret: String

    // validade do access token em minutos (ex: 15 min)
    var accessTokenValidityMinutes: Long = 15

    // validade do refresh token em minutos (ex: 7 dias)
    var refreshTokenValidityMinutes: Long = 60 * 24 * 7

    @Bean
    fun jwtSecretKey(): SecretKey {
        require(secret.isNotBlank()) {
            "Missing JWT secret. Configure 'security.jwt.secret' or env var 'JWT_SECRET'."
        }
        // HS256: ideal ter pelo menos 32 bytes
        require(secret.toByteArray(Charsets.UTF_8).size >= 32) {
            "JWT secret is too short for HS256. Use at least 32 bytes (recommended 64+)."
        }

        // HS256 = HmacSHA256
        val keyBytes = secret.toByteArray(Charsets.UTF_8)
        return SecretKeySpec(keyBytes, "HmacSHA256")
    }

    @Bean
    fun jwtDecoder(secretKey: SecretKey): JwtDecoder {
        // usado pelo Resource Server para VALIDAR tokens recebidos
        return NimbusJwtDecoder.withSecretKey(secretKey).build()
    }

    @Bean
    fun jwtEncoder(secretKey: SecretKey): JwtEncoder {
        val jwk = OctetSequenceKey.Builder(secretKey.encoded)
            .keyUse(KeyUse.SIGNATURE)
            .algorithm(JWSAlgorithm.HS256)
            .build()

        val jwkSet = JWKSet(jwk)
        val jwkSource = ImmutableJWKSet<SecurityContext>(jwkSet)

        return NimbusJwtEncoder(jwkSource)
    }
}
