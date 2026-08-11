package kz.bardak.auth.api;

import jakarta.validation.Valid;
import java.util.Objects;
import java.util.UUID;
import kz.bardak.auth.AuthService;
import kz.bardak.auth.ws.WsTicketService;
import org.springframework.http.HttpHeaders;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.security.oauth2.jwt.Jwt;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestHeader;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;
import org.springframework.security.core.annotation.AuthenticationPrincipal;

/**
 * Регистрация, вход, обновление сессии и выдача WS-тикета.
 *
 * <p>{@code User-Agent} сохраняется вместе с refresh-токеном: по нему потом видно,
 * с какого устройства сессия, и какую именно отзывать.
 */
@RestController
@RequestMapping("/api/auth")
public class AuthController {

    private final AuthService authService;
    private final WsTicketService wsTickets;

    public AuthController(final AuthService authService, final WsTicketService wsTickets) {
        this.authService = Objects.requireNonNull(authService, "authService");
        this.wsTickets = Objects.requireNonNull(wsTickets, "wsTickets");
    }

    @PostMapping("/register")
    public AuthDtos.TokenPair register(@Valid @RequestBody final AuthDtos.RegisterRequest request,
                                       @RequestHeader(value = HttpHeaders.USER_AGENT, required = false)
                                       final String userAgent) {
        return authService.register(request, userAgent);
    }

    @PostMapping("/login")
    public AuthDtos.TokenPair login(@Valid @RequestBody final AuthDtos.LoginRequest request,
                                    @RequestHeader(value = HttpHeaders.USER_AGENT, required = false)
                                    final String userAgent) {
        return authService.login(request, userAgent);
    }

    @PostMapping("/refresh")
    public AuthDtos.TokenPair refresh(@Valid @RequestBody final AuthDtos.RefreshRequest request,
                                      @RequestHeader(value = HttpHeaders.USER_AGENT, required = false)
                                      final String userAgent) {
        return authService.refresh(request.refreshToken(), userAgent);
    }

    @PostMapping("/logout")
    public ResponseEntity<Void> logout(@Valid @RequestBody final AuthDtos.RefreshRequest request) {
        authService.logout(request.refreshToken());
        return ResponseEntity.status(HttpStatus.NO_CONTENT).build();
    }

    /** Тикет выдаётся уже авторизованному: это обмен access-токена на право открыть сокет. */
    @PostMapping("/ws-ticket")
    public AuthDtos.WsTicket wsTicket(@AuthenticationPrincipal final Jwt jwt) {
        return new AuthDtos.WsTicket(wsTickets.issue(UUID.fromString(jwt.getSubject())),
                wsTickets.ttlSeconds());
    }
}
