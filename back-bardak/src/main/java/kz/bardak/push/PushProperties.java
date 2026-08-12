package kz.bardak.push;

import java.time.Duration;
import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Настройки push-уведомлений.
 *
 * @param publicKey  открытый ключ VAPID (base64url) — его же получает браузер при подписке
 * @param privateKey закрытый ключ VAPID; в проде обязателен через окружение
 * @param subject    контакт владельца сервиса: {@code mailto:} или адрес сайта. Требование
 *                   RFC 8292 — по нему push-сервис связывается, если отправка мешает
 * @param quietFor   ⭐ не слать уведомление, если ход перешёл к игроку и тут же вернулся:
 *                   в быстрой партии это дало бы очередь звонков вместо игры
 */
@ConfigurationProperties(prefix = "bardak.push")
public record PushProperties(String publicKey, String privateKey, String subject,
                             Duration quietFor) {

    public PushProperties {
        quietFor = quietFor == null ? Duration.ofMinutes(2) : quietFor;
        subject = subject == null || subject.isBlank() ? "mailto:admin@bardak.local" : subject;
    }

    /**
     * Уведомления включены. Без ключей их отправлять нечем — и это не поломка: локально
     * играют с открытой вкладкой, и заводить ключи ради этого незачем.
     */
    public boolean isEnabled() {
        return publicKey != null && !publicKey.isBlank()
                && privateKey != null && !privateKey.isBlank();
    }
}
