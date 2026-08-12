package kz.bardak.push.api;

import jakarta.validation.Valid;
import jakarta.validation.constraints.NotBlank;
import java.util.Map;
import java.util.Objects;
import java.util.UUID;
import kz.bardak.push.PushSubscriptionService;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.security.core.annotation.AuthenticationPrincipal;
import org.springframework.security.oauth2.jwt.Jwt;
import org.springframework.web.bind.annotation.DeleteMapping;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestHeader;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

/** Подписка на уведомления «твой ход». */
@RestController
@RequestMapping("/api/push")
public class PushController {

    private final PushSubscriptionService subscriptions;

    public PushController(final PushSubscriptionService subscriptions) {
        this.subscriptions = Objects.requireNonNull(subscriptions, "subscriptions");
    }

    /**
     * Открытый ключ VAPID для браузера.
     *
     * <p>{@code enabled: false} — уведомления не настроены; клиент по этому признаку
     * не показывает кнопку подписки, а не показывает ошибку: ключей может не быть
     * намеренно.
     */
    @GetMapping("/key")
    public Map<String, Object> key() {
        final String publicKey = subscriptions.publicKey();
        return publicKey == null
                ? Map.of("enabled", false)
                : Map.of("enabled", true, "publicKey", publicKey);
    }

    @PostMapping("/subscriptions")
    public ResponseEntity<Void> subscribe(@Valid @RequestBody final SubscribeRequest request,
                                          @RequestHeader(value = "User-Agent", required = false)
                                          final String userAgent,
                                          @AuthenticationPrincipal final Jwt jwt) {
        subscriptions.subscribe(UUID.fromString(jwt.getSubject()), request.endpoint(),
                request.p256dh(), request.auth(), userAgent);
        return ResponseEntity.status(HttpStatus.NO_CONTENT).build();
    }

    @DeleteMapping("/subscriptions")
    public ResponseEntity<Void> unsubscribe(@Valid @RequestBody final UnsubscribeRequest request) {
        subscriptions.unsubscribe(request.endpoint());
        return ResponseEntity.status(HttpStatus.NO_CONTENT).build();
    }

    /** Ключи подписки браузер отдаёт в base64url; сервер их не разбирает, а только хранит. */
    public record SubscribeRequest(@NotBlank String endpoint, @NotBlank String p256dh,
                                   @NotBlank String auth) {
    }

    public record UnsubscribeRequest(@NotBlank String endpoint) {
    }
}
