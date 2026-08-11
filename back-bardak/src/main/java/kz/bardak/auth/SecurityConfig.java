package kz.bardak.auth;

import com.nimbusds.jose.jwk.source.ImmutableSecret;
import java.nio.charset.StandardCharsets;
import java.time.Clock;
import javax.crypto.spec.SecretKeySpec;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.http.HttpMethod;
import org.springframework.security.config.annotation.web.builders.HttpSecurity;
import org.springframework.security.config.annotation.web.configuration.EnableWebSecurity;
import org.springframework.security.config.http.SessionCreationPolicy;
import org.springframework.security.crypto.bcrypt.BCryptPasswordEncoder;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.security.oauth2.jose.jws.MacAlgorithm;
import org.springframework.security.oauth2.jwt.JwtDecoder;
import org.springframework.security.oauth2.jwt.JwtEncoder;
import org.springframework.security.oauth2.jwt.NimbusJwtDecoder;
import org.springframework.security.oauth2.jwt.NimbusJwtEncoder;
import org.springframework.security.web.SecurityFilterChain;
import org.springframework.scheduling.annotation.EnableScheduling;

/**
 * Безопасность REST: без сессий, только токен в заголовке.
 *
 * <p>CSRF выключен осознанно: состояния в сессии нет, а access-токен браузер сам никуда
 * не подставляет — подделать межсайтовый запрос нечем.
 *
 * <p>Авторизация WebSocket живёт отдельно (ADR-005): браузерный сокет не умеет слать
 * заголовок, поэтому рукопожатие проверяет одноразовый тикет.
 */
@Configuration
@EnableWebSecurity
@EnableScheduling
@EnableConfigurationProperties({AuthProperties.class, kz.bardak.game.runtime.GameProperties.class})
public class SecurityConfig {

    @Bean
    public SecurityFilterChain securityFilterChain(final HttpSecurity http) throws Exception {
        return http
                .csrf(csrf -> csrf.disable())
                .sessionManagement(session -> session.sessionCreationPolicy(SessionCreationPolicy.STATELESS))
                .authorizeHttpRequests(requests -> requests
                        .requestMatchers(HttpMethod.POST, "/api/auth/register", "/api/auth/login",
                                "/api/auth/refresh", "/api/auth/logout").permitAll()
                        .requestMatchers("/api/health", "/actuator/**", "/assets/**").permitAll()
                        // ⭐ Сокет пропускается фильтром намеренно: браузерный WebSocket не умеет
                        // слать Authorization, поэтому его авторизует не Spring Security,
                        // а WsTicketHandshakeInterceptor по одноразовому тикету (ADR-005).
                        .requestMatchers("/ws").permitAll()
                        .requestMatchers(HttpMethod.GET, "/", "/index.html", "/app/**", "/*.css",
                                "/*.js", "/*.ico", "/*.png", "/*.svg").permitAll()
                        .anyRequest().authenticated())
                .oauth2ResourceServer(oauth2 -> oauth2.jwt(jwt -> {
                }))
                .build();
    }

    @Bean
    public JwtEncoder jwtEncoder(final AuthProperties properties) {
        return new NimbusJwtEncoder(new ImmutableSecret<>(secretKey(properties)));
    }

    @Bean
    public JwtDecoder jwtDecoder(final AuthProperties properties) {
        return NimbusJwtDecoder.withSecretKey(secretKey(properties))
                .macAlgorithm(MacAlgorithm.HS256)
                .build();
    }

    @Bean
    public PasswordEncoder passwordEncoder() {
        return new BCryptPasswordEncoder();
    }

    /** Часы отдельным бином: «сейчас» в тестах должно быть управляемым. */
    @Bean
    public Clock clock() {
        return Clock.systemUTC();
    }

    private SecretKeySpec secretKey(final AuthProperties properties) {
        return new SecretKeySpec(properties.jwtSecret().getBytes(StandardCharsets.UTF_8), "HmacSHA256");
    }
}
