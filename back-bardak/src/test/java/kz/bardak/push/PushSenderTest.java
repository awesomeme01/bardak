package kz.bardak.push;

import static org.assertj.core.api.Assertions.assertThat;

import com.fasterxml.jackson.databind.ObjectMapper;
import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.time.ZoneOffset;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

/** Текст уведомления и поведение без настроенных ключей. */
@ExtendWith(MockitoExtension.class)
class PushSenderTest {

    @Mock
    private kz.bardak.push.domain.PushSubscriptionRepository subscriptions;

    @DisplayName("Should name the table in the notification When the table has a name")
    @Test
    void shouldNameTheTableInTheNotificationWhenTheTableHasAName() {
        final String payload = senderWithoutKeys().turnPayload("Вечерний", "table-1");

        assertThat(payload).contains("Твой ход").contains("Вечерний").contains("table-1");
    }

    @DisplayName("Should fall back to a generic text When the table has no name")
    @Test
    void shouldFallBackToAGenericTextWhenTheTableHasNoName() {
        final String payload = senderWithoutKeys().turnPayload("  ", null);

        // ⭐ Пустое название лучше заменить фразой, чем показать «Стол «» ждёт».
        assertThat(payload).contains("За столом ждут тебя").doesNotContain("tableId");
    }

    @DisplayName("Should say how long is left When the match is paused")
    @Test
    void shouldSayHowLongIsLeftWhenTheMatchIsPaused() {
        final String payload = senderWithoutKeys().pausedPayload("Вечерний", "table-1", 60);

        // Человеку важно не «матч на паузе», а сколько у него есть, чтобы вернуться.
        assertThat(payload).contains("Тебя ждут").contains("Вечерний").contains("60");
    }

    @DisplayName("Should stay disabled When VAPID keys are not configured")
    @Test
    void shouldStayDisabledWhenVapidKeysAreNotConfigured() {
        // Отсутствие ключей — не поломка: локально играют с открытой вкладкой.
        assertThat(senderWithoutKeys().isEnabled()).isFalse();
    }

    private PushSender senderWithoutKeys() {
        return new PushSender(subscriptions,
                new PushProperties(null, null, null, Duration.ofMinutes(2)),
                new ObjectMapper(), Clock.fixed(Instant.EPOCH, ZoneOffset.UTC));
    }
}
