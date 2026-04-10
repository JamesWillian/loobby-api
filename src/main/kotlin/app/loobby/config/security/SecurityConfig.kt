package app.loobby.config.security

import org.springframework.context.annotation.Bean
import org.springframework.context.annotation.Configuration
import org.springframework.http.HttpMethod
import org.springframework.security.config.Customizer.withDefaults
import org.springframework.security.config.annotation.web.builders.HttpSecurity
import org.springframework.security.config.http.SessionCreationPolicy
import org.springframework.security.web.SecurityFilterChain

@Configuration
class SecurityConfig {

    @Bean
    fun securityFilterChain(
        http: HttpSecurity,
        jwtAuthConverter: JwtAuthConverter
    ): SecurityFilterChain {
        return http
            // API stateless geralmente não usa CSRF (CSRF é mais pra browser com cookie/sessão)
            .csrf { it.disable() }

            // garante que o Spring Security não crie/guarde sessão
            .sessionManagement { it.sessionCreationPolicy(SessionCreationPolicy.STATELESS) }

            // regras de autorização por rota
            .authorizeHttpRequests { auth ->
                auth
                    // auth público
                    .requestMatchers(HttpMethod.POST, "/auth/anonymous").permitAll()
                    .requestMatchers(HttpMethod.POST, "/auth/refresh").permitAll()
                    .requestMatchers(HttpMethod.POST, "/auth/login").permitAll()
                    .requestMatchers(HttpMethod.GET, "/auth/verify-email").permitAll()
                    .requestMatchers(HttpMethod.POST, "/auth/forgot-password").permitAll()
                    .requestMatchers(HttpMethod.GET, "/auth/reset-password").permitAll()
                    .requestMatchers(HttpMethod.POST, "/auth/reset-password").permitAll()
                    .requestMatchers(HttpMethod.GET, "/files/**").permitAll()

                    // register: precisa estar logado (anônimo ou real)
                    .requestMatchers(HttpMethod.POST, "/auth/register").authenticated()

                    // health check
                    .requestMatchers(HttpMethod.GET, "/actuator/health").permitAll()

                    // qualquer outra rota precisa de JWT válido
                    .anyRequest().authenticated()
            }

            // habilita "Resource Server" com JWT (vai ler Authorization: Bearer ...)
            .oauth2ResourceServer { rs ->
                rs.jwt { jwt ->
                    jwt.jwtAuthenticationConverter(jwtAuthConverter)
                }
            }

            // defaults úteis (headers básicos etc.)
            .httpBasic { it.disable() } // não usar basic auth
            .formLogin { it.disable() } // não usar form login
            .cors(withDefaults())       // habilita CORS (configure um CorsConfigurationSource se precisar)
            .build()
    }
}
