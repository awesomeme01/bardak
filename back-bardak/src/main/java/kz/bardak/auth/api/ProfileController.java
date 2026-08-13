package kz.bardak.auth.api;

import jakarta.validation.Valid;
import java.util.Objects;
import java.util.UUID;
import kz.bardak.auth.AuthService;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.security.core.annotation.AuthenticationPrincipal;
import org.springframework.security.oauth2.jwt.Jwt;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PatchMapping;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

/** Свой профиль: посмотреть и поправить. */
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

    /** Имя за столом и мордочка. Логин не меняется: по нему входят. */
    @PatchMapping
    public AuthDtos.UserView update(@Valid @RequestBody final AuthDtos.UpdateProfileRequest request,
                                    @AuthenticationPrincipal final Jwt jwt) {
        return authService.updateProfile(UUID.fromString(jwt.getSubject()),
                request.displayName(), request.avatar());
    }

    /**
     * Смена пароля.
     *
     * <p>⚠️ Все входы гасятся: смена пароля и означает «выкинуть тех, кто вошёл раньше».
     * Текущая вкладка живёт до конца access-токена, а потом попросит войти заново.
     */
    @PostMapping("/password")
    public ResponseEntity<Void> changePassword(
            @Valid @RequestBody final AuthDtos.ChangePasswordRequest request,
            @AuthenticationPrincipal final Jwt jwt) {
        authService.changePassword(UUID.fromString(jwt.getSubject()),
                request.currentPassword(), request.newPassword());
        return ResponseEntity.status(HttpStatus.NO_CONTENT).build();
    }
}
