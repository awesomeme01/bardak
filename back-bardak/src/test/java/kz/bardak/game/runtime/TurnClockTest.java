package kz.bardak.game.runtime;

import static org.assertj.core.api.Assertions.assertThat;

import java.time.Duration;
import java.util.UUID;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

/**
 * Таймер хода (§5.2).
 *
 * <p>⭐ Главное здесь — что пауза <b>останавливает</b> отсчёт, а не сбрасывает его:
 * игрок, у которого оставалось чуть-чуть, после возвращения получает своё «чуть-чуть»,
 * а не полный ход заново.
 */
class TurnClockTest {

    private final TurnClock clock = new TurnClock();

    @DisplayName("Should fire the action When the turn runs out")
    @Test
    void shouldFireTheActionWhenTheTurnRunsOut() throws Exception {
        final CountDownLatch fired = new CountDownLatch(1);

        clock.start(UUID.randomUUID(), Duration.ofMillis(150), fired::countDown);

        assertThat(fired.await(2, TimeUnit.SECONDS)).isTrue();
    }

    @DisplayName("Should not fire anything When the timer is cancelled")
    @Test
    void shouldNotFireAnythingWhenTheTimerIsCancelled() throws Exception {
        final UUID tableId = UUID.randomUUID();
        final CountDownLatch fired = new CountDownLatch(1);
        clock.start(tableId, Duration.ofMillis(200), fired::countDown);

        clock.cancel(tableId);

        assertThat(fired.await(600, TimeUnit.MILLISECONDS)).isFalse();
    }

    @DisplayName("Should keep the remainder When the timer is paused and resumed")
    @Test
    void shouldKeepTheRemainderWhenTheTimerIsPausedAndResumed() throws Exception {
        final UUID tableId = UUID.randomUUID();
        final CountDownLatch fired = new CountDownLatch(1);
        clock.start(tableId, Duration.ofMillis(600), fired::countDown);
        Thread.sleep(400);

        final Duration left = clock.pause(tableId);

        assertThat(left).isLessThan(Duration.ofMillis(400));
        assertThat(fired.await(500, TimeUnit.MILLISECONDS))
                .withFailMessage("На паузе таймер не должен срабатывать")
                .isFalse();

        clock.resume(tableId);

        // ⭐ После продолжения остаётся именно остаток: полный ход не начинается заново.
        assertThat(fired.await(500, TimeUnit.MILLISECONDS)).isTrue();
    }

    @DisplayName("Should replace the previous timer When a new turn starts")
    @Test
    void shouldReplaceThePreviousTimerWhenANewTurnStarts() throws Exception {
        final UUID tableId = UUID.randomUUID();
        final CountDownLatch firstFired = new CountDownLatch(1);
        final CountDownLatch secondFired = new CountDownLatch(1);

        clock.start(tableId, Duration.ofMillis(200), firstFired::countDown);
        clock.start(tableId, Duration.ofMillis(400), secondFired::countDown);

        assertThat(secondFired.await(2, TimeUnit.SECONDS)).isTrue();
        assertThat(firstFired.getCount())
                .withFailMessage("Старый таймер обязан быть отменён")
                .isEqualTo(1);
    }

    @DisplayName("Should stop waiting for the player When the abort timer is cancelled")
    @Test
    void shouldStopWaitingForThePlayerWhenTheAbortTimerIsCancelled() throws Exception {
        final UUID tableId = UUID.randomUUID();
        final CountDownLatch aborted = new CountDownLatch(1);
        clock.scheduleAbort(tableId, Duration.ofMillis(200), aborted::countDown);

        clock.cancelAbort(tableId);

        assertThat(aborted.await(500, TimeUnit.MILLISECONDS)).isFalse();
    }
}
