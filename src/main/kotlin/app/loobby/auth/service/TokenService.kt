package app.loobby.auth.service

import app.loobby.config.security.JwtConfig
import org.springframework.security.oauth2.jose.jws.MacAlgorithm
import org.springframework.security.oauth2.jwt.JwsHeader
import org.springframework.security.oauth2.jwt.JwtClaimsSet
import org.springframework.security.oauth2.jwt.JwtEncoder
import org.springframework.security.oauth2.jwt.JwtEncoderParameters
import org.springframework.stereotype.Service
import java.time.Instant
import java.time.temporal.ChronoUnit
import java.util.UUID

@Service
class TokenService(
    private val jwtEncoder: JwtEncoder,
    private val jwtConfig: JwtConfig
) {

    fun generateAccessToken(
        userId: UUID,
        username: String,
        roles: List<String>
    ): String {
        return generateToken(
            userId = userId,
            username = username,
            roles = roles,
            expiresInMinutes = jwtConfig.accessTokenValidityMinutes,
            type = "access"
        )
    }

    fun generateRefreshToken(
        userId: UUID
    ): String {
        // refresh não precisa de username/roles; só vínculo com o usuário
        return generateToken(
            userId = userId,
            username = null,
            roles = null,
            expiresInMinutes = jwtConfig.refreshTokenValidityMinutes,
            type = "refresh"
        )
    }

    private fun generateToken(
        userId: UUID,
        username: String?,
        roles: List<String>?,
        expiresInMinutes: Long,
        type: String
    ): String {
        val now = Instant.now()
        val exp = now.plus(expiresInMinutes, ChronoUnit.MINUTES)

        val claimsBuilder = JwtClaimsSet.builder()
            .subject(userId.toString())
            .issuedAt(now)
            .expiresAt(exp)
            .claim("type", type)

        if (username != null) {
            claimsBuilder.claim("username", username)
        }

        if (roles != null) {
            claimsBuilder.claim("roles", roles)
        }

        val claims = claimsBuilder.build()

        val header = JwsHeader.with(MacAlgorithm.HS256).build()
        val params = JwtEncoderParameters.from(header, claims)

        return jwtEncoder.encode(params).tokenValue
    }

}
