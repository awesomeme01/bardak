package kz.bardak.auth;

import java.time.Clock;
import java.time.Instant;
import java.util.Objects;
import kz.bardak.auth.domain.User;
import org.springframework.security.oauth2.jose.jws.MacAlgorithm;
import org.springframework.security.oauth2.jwt.JwsHeader;
import org.springframework.security.oauth2.jwt.JwtClaimsSet;
import org.springframework.security.oauth2.jwt.JwtEncoder;
import org.springframework.security.oauth2.jwt.JwtEncoderParameters;
import org.springframework.stereotype.Service;

/**
 * Выпуск access-токенов. Разбор и проверка подписи — забота Spring Security
 * (`oauth2-resource-server`): свой парсер JWT здесь был бы лишним источником дыр.
 */
@Service
public class AccessTokenService {

    private final JwtEncoder encoder;
    private final AuthProperties properties;
    private final Clock clock;

    public AccessTokenService(final JwtEncoder encoder, final AuthProperties properties, final Clock clock) {
        this.encoder = Objects.requireNonNull(encoder, "encoder");
        this.properties = Objects.requireNonNull(properties, "properties");
        this.clock = Objects.requireNonNull(clock, "clock");
    }

    /**
     * Токен для игрока. В claims только то, что нужно серверу для авторизации:
     * идентификатор и отображаемое имя. Ролей в игре нет.
     */
    public String issue(final User user) {
        Objects.requireNonNull(user, "user");
        final Instant now = clock.instant();
        final JwtClaimsSet claims = JwtClaimsSet.builder()
                .issuer("bardak")
                .issuedAt(now)
                .expiresAt(now.plus(properties.accessTtl()))
                .subject(user.id().toString())
                .claim("username", user.username())
                .claim("displayName", user.displayName())
                .build();
        // Алгоритм указывается явно: по умолчанию Nimbus ищет ключ под RS256, а у нас HMAC.
        final JwsHeader header = JwsHeader.with(MacAlgorithm.HS256).build();
        return encoder.encode(JwtEncoderParameters.from(header, claims)).getTokenValue();
    }

    public long accessTtlSeconds() {
        return properties.accessTtl().toSeconds();
    }
}
