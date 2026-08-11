package kz.bardak.auth;

import java.util.Objects;
import java.util.UUID;
import kz.bardak.auth.api.AuthDtos;
import kz.bardak.auth.domain.User;
import kz.bardak.auth.domain.UserRepository;
import kz.bardak.common.web.ApiException;
import org.springframework.http.HttpStatus;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

/**
 * Регистрация, вход и обновление сессии.
 *
 * <p>⚠️ Вход отвечает одинаково и на неверный пароль, и на несуществующий логин: иначе
 * по ответам перебираются имена зарегистрированных игроков.
 */
@Service
public class AuthService {

    private final UserRepository users;
    private final PasswordEncoder passwordEncoder;
    private final AccessTokenService accessTokens;
    private final RefreshTokenService refreshTokens;
    private final AuthProperties properties;

    public AuthService(final UserRepository users, final PasswordEncoder passwordEncoder,
                       final AccessTokenService accessTokens, final RefreshTokenService refreshTokens,
                       final AuthProperties properties) {
        this.users = Objects.requireNonNull(users, "users");
        this.passwordEncoder = Objects.requireNonNull(passwordEncoder, "passwordEncoder");
        this.accessTokens = Objects.requireNonNull(accessTokens, "accessTokens");
        this.refreshTokens = Objects.requireNonNull(refreshTokens, "refreshTokens");
        this.properties = Objects.requireNonNull(properties, "properties");
    }

    @Transactional
    public AuthDtos.TokenPair register(final AuthDtos.RegisterRequest request, final String userAgent) {
        if (!properties.isInviteCodeValid(request.inviteCode())) {
            throw new ApiException(HttpStatus.FORBIDDEN, "INVALID_INVITE_CODE",
                    "Неверный код приглашения");
        }
        if (users.existsByUsername(request.username())) {
            throw new ApiException(HttpStatus.CONFLICT, "USERNAME_TAKEN", "Логин уже занят");
        }
        final User user = users.save(new User(UUID.randomUUID(), request.username(), request.displayName(),
                request.email(), passwordEncoder.encode(request.password())));
        return issuePair(user, userAgent);
    }

    @Transactional
    public AuthDtos.TokenPair login(final AuthDtos.LoginRequest request, final String userAgent) {
        final User user = users.findByUsername(request.username())
                .filter(candidate -> passwordEncoder.matches(request.password(), candidate.passwordHash()))
                .filter(User::isActive)
                .orElseThrow(() -> new ApiException(HttpStatus.UNAUTHORIZED, "INVALID_CREDENTIALS",
                        "Неверный логин или пароль"));
        return issuePair(user, userAgent);
    }

    @Transactional
    public AuthDtos.TokenPair refresh(final String refreshToken, final String userAgent) {
        final UUID userId = refreshTokens.rotate(refreshToken, userAgent);
        final User user = users.findById(userId)
                .filter(User::isActive)
                .orElseThrow(() -> new ApiException(HttpStatus.UNAUTHORIZED, "REFRESH_TOKEN_INVALID",
                        "Сессия истекла, войдите заново"));
        return issuePair(user, userAgent);
    }

    @Transactional
    public void logout(final String refreshToken) {
        refreshTokens.revoke(refreshToken);
    }

    @Transactional(readOnly = true)
    public AuthDtos.UserView profile(final UUID userId) {
        return users.findById(userId)
                .map(AuthService::toView)
                .orElseThrow(() -> new ApiException(HttpStatus.NOT_FOUND, "USER_NOT_FOUND",
                        "Пользователь не найден"));
    }

    private AuthDtos.TokenPair issuePair(final User user, final String userAgent) {
        return new AuthDtos.TokenPair(
                accessTokens.issue(user),
                refreshTokens.issue(user.id(), userAgent),
                accessTokens.accessTtlSeconds(),
                toView(user));
    }

    private static AuthDtos.UserView toView(final User user) {
        return new AuthDtos.UserView(user.id().toString(), user.username(),
                user.displayName(), user.avatarUrl());
    }
}
