package kz.bardak.push;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyLong;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.times;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.UUID;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

/**
 * Кому звонить «твой ход».
 *
 * <p>⭐ Проверяется прежде всего молчание: лишний звонок хуже пропущенного. Игрок,
 * сидящий за столом, видит ход сам, а частые звонки заканчиваются тем, что уведомления
 * отключают целиком — и тогда не приходит уже ничего.
 */
@ExtendWith(MockitoExtension.class)
class TurnNotifierTest {

    private static final UUID PLAYER = UUID.randomUUID();
    private static final UUID TABLE = UUID.randomUUID();
    private static final Instant NOON = Instant.parse("2026-08-11T12:00:00Z");

    @Mock
    private PushSender sender;

    @DisplayName("Should call the player When it is their turn and they are away")
    @Test
    void shouldCallThePlayerWhenItIsTheirTurnAndTheyAreAway() {
        when(sender.isEnabled()).thenReturn(true);
        final TurnNotifier notifier = notifierAt(NOON);

        notifier.turnOf(PLAYER, "Вечерний", TABLE, false);

        verify(sender).notifyTurn(eq(PLAYER), eq("Вечерний"), eq(TABLE.toString()));
    }

    @DisplayName("Should stay silent When the player is at the table")
    @Test
    void shouldStaySilentWhenThePlayerIsAtTheTable() {
        // ⭐ Отправитель включён намеренно: иначе тест прошёл бы по совсем другой причине
        // и не заметил бы, если проверку присутствия убрать вовсе.
        when(sender.isEnabled()).thenReturn(true);
        final TurnNotifier notifier = notifierAt(NOON);

        notifier.turnOf(PLAYER, "Вечерний", TABLE, true);

        verify(sender, never()).notifyTurn(any(), anyString(), anyString());
    }

    @DisplayName("Should call only once When the turn comes back within the quiet window")
    @Test
    void shouldCallOnlyOnceWhenTheTurnComesBackWithinTheQuietWindow() {
        when(sender.isEnabled()).thenReturn(true);
        final MutableClock clock = new MutableClock(NOON);
        final TurnNotifier notifier = new TurnNotifier(sender, properties(Duration.ofMinutes(2)), clock);

        notifier.turnOf(PLAYER, "Вечерний", TABLE, false);
        clock.advance(Duration.ofSeconds(30));
        notifier.turnOf(PLAYER, "Вечерний", TABLE, false);

        // ⭐ Ход вернулся через полминуты — это одна и та же партия, а не второй повод звонить.
        verify(sender, times(1)).notifyTurn(any(), anyString(), anyString());
    }

    @DisplayName("Should call again When the quiet window is over")
    @Test
    void shouldCallAgainWhenTheQuietWindowIsOver() {
        when(sender.isEnabled()).thenReturn(true);
        final MutableClock clock = new MutableClock(NOON);
        final TurnNotifier notifier = new TurnNotifier(sender, properties(Duration.ofMinutes(2)), clock);

        notifier.turnOf(PLAYER, "Вечерний", TABLE, false);
        clock.advance(Duration.ofMinutes(3));
        notifier.turnOf(PLAYER, "Вечерний", TABLE, false);

        verify(sender, times(2)).notifyTurn(any(), anyString(), anyString());
    }

    @DisplayName("Should call right away When the player came back and left again")
    @Test
    void shouldCallRightAwayWhenThePlayerCameBackAndLeftAgain() {
        when(sender.isEnabled()).thenReturn(true);
        final MutableClock clock = new MutableClock(NOON);
        final TurnNotifier notifier = new TurnNotifier(sender, properties(Duration.ofMinutes(2)), clock);
        notifier.turnOf(PLAYER, "Вечерний", TABLE, false);

        notifier.present(PLAYER);
        clock.advance(Duration.ofSeconds(5));
        notifier.turnOf(PLAYER, "Вечерний", TABLE, false);

        // Игрок заходил и снова пропал: окно тишины считается от последнего звонка,
        // но возвращение его обнуляет — иначе он не узнает о ходе ещё две минуты.
        verify(sender, times(2)).notifyTurn(any(), anyString(), anyString());
    }

    @DisplayName("Should call the player back When the match paused because of them")
    @Test
    void shouldCallThePlayerBackWhenTheMatchPausedBecauseOfThem() {
        when(sender.isEnabled()).thenReturn(true);
        final TurnNotifier notifier = notifierAt(NOON);

        notifier.pausedFor(PLAYER, "Вечерний", TABLE, 60);

        verify(sender).notifyPaused(eq(PLAYER), eq("Вечерний"), eq(TABLE.toString()), eq(60L));
    }

    @DisplayName("Should ignore the quiet window When the match is on pause")
    @Test
    void shouldIgnoreTheQuietWindowWhenTheMatchIsOnPause() {
        when(sender.isEnabled()).thenReturn(true);
        final MutableClock clock = new MutableClock(NOON);
        final TurnNotifier notifier = new TurnNotifier(sender, properties(Duration.ofMinutes(2)), clock);
        notifier.turnOf(PLAYER, "Вечерний", TABLE, false);

        clock.advance(Duration.ofSeconds(10));
        notifier.pausedFor(PLAYER, "Вечерний", TABLE, 60);

        // ⭐ Цена молчания здесь — отменённый матч у всех за столом, а не лишний звонок.
        verify(sender).notifyPaused(any(), anyString(), anyString(), anyLong());
    }

    @DisplayName("Should do nothing When push is not configured")
    @Test
    void shouldDoNothingWhenPushIsNotConfigured() {
        when(sender.isEnabled()).thenReturn(false);
        final TurnNotifier notifier = notifierAt(NOON);

        notifier.turnOf(PLAYER, "Вечерний", TABLE, false);

        verify(sender, never()).notifyTurn(any(), anyString(), anyString());
    }

    private TurnNotifier notifierAt(final Instant now) {
        return new TurnNotifier(sender, properties(Duration.ofMinutes(2)),
                Clock.fixed(now, ZoneOffset.UTC));
    }

    private PushProperties properties(final Duration quietFor) {
        return new PushProperties("public", "private", "mailto:test@bardak.local", quietFor);
    }

    /** Часы, которые можно двигать: окно тишины иначе не проверить. */
    private static final class MutableClock extends Clock {

        private Instant now;

        private MutableClock(final Instant start) {
            this.now = start;
        }

        private void advance(final Duration duration) {
            now = now.plus(duration);
        }

        @Override
        public ZoneOffset getZone() {
            return ZoneOffset.UTC;
        }

        @Override
        public Clock withZone(final java.time.ZoneId zone) {
            return this;
        }

        @Override
        public Instant instant() {
            return now;
        }
    }
}
