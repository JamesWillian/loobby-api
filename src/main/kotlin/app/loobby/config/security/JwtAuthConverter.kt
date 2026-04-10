package app.loobby.config.security

import org.springframework.core.convert.converter.Converter
import org.springframework.security.authentication.AbstractAuthenticationToken
import org.springframework.security.core.authority.SimpleGrantedAuthority
import org.springframework.security.oauth2.jwt.Jwt
import org.springframework.security.oauth2.server.resource.authentication.JwtAuthenticationToken
import org.springframework.stereotype.Component

@Component
class JwtAuthConverter : Converter<Jwt, AbstractAuthenticationToken> {

    override fun convert(jwt: Jwt): AbstractAuthenticationToken {
        val authorities = extractAuthorities(jwt)
        // principalName padrão do JwtAuthenticationToken = jwt.subject (sub)
        return JwtAuthenticationToken(jwt, authorities, jwt.subject ?: "")
    }

    private fun extractAuthorities(jwt: Jwt): List<SimpleGrantedAuthority> {
        val raw = jwt.getClaim<Any>("roles") ?: return emptyList()

        val roles: List<String> = when (raw) {
            is String -> raw
                .split(",", " ")
                .map { it.trim() }
                .filter { it.isNotBlank() }
            is Collection<*> -> raw.mapNotNull { it?.toString() }
            else -> emptyList()
        }

        return roles
            .map { it.trim() }
            .filter { it.isNotBlank() }
            .map { role ->
                val normalized = role
                    .removePrefix("ROLE_")
                    .uppercase()

                SimpleGrantedAuthority("ROLE_$normalized")
            }
    }
}
