package com.jammes.loobby.auth.service

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
    private val jwtEncoder: JwtEncoder
) {

    fun generateToken(
        userId: UUID,
        username: String,
        roles: List<String>,
        expiresInMinutes: Long = 60 * 24 * 7
    ): String {

        val now = Instant.now()
        val exp = now.plus(expiresInMinutes, ChronoUnit.MINUTES)

        val claims = JwtClaimsSet.builder()
            .subject(userId.toString())
            .claim("username", username)
            .claim("roles", roles)
            .issuedAt(now)
            .expiresAt(exp)
            .build()

        // 🔥 Header explícito COM algoritmo HS256
        val header = JwsHeader.with(MacAlgorithm.HS256).build()

        val params = JwtEncoderParameters.from(header, claims)

        return jwtEncoder.encode(params).tokenValue
    }
}
