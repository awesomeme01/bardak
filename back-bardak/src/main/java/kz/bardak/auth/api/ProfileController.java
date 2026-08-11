package kz.bardak.auth.api;

import java.util.Objects;
import java.util.UUID;
import kz.bardak.auth.AuthService;
import org.springframework.security.core.annotation.AuthenticationPrincipal;
import org.springframework.security.oauth2.jwt.Jwt;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

/** Первый защищённый эндпоинт: по нему проверяется, что авторизация вообще работает. */
@RestController
@RequestMapping("/api/profile")
public class ProfileController {

    private final AuthService authService;

    public ProfileController(final AuthService authService) {
        this.authService = Objects.requireNonNull(authService, "authService");
    }

    @GetMapping
    public AuthDtos.UserView profile(@AuthenticationPrincipal final Jwt jwt) {
        return authService.profile(UUID.fromString(jwt.getSubject()));
    }
}
