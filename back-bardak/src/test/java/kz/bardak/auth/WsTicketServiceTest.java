package kz.bardak.auth;

import static org.assertj.core.api.Assertions.assertThat;

import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.List;
import java.util.UUID;
import kz.bardak.auth.ws.WsTicketService;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

/**
 * Одноразовые WS-тикеты (ADR-005).
 */
class WsTicketServiceTest {

    private static final Instant NOW = Instant.parse("2026-08-11T10:00:00Z");
    private static final UUID USER = UUID.fromString("00000000-0000-0000-0000-0000000000bb");

    @DisplayName("Should return the player When a fresh ticket is consumed")
    @Test
    void shouldReturnThePlayerWhenAFreshTicketIsConsumed() {
        final WsTicketService service = serviceAt(NOW);

        assertThat(service.consume(service.issue(USER))).contains(USER);
    }

    @DisplayName("Should burn the ticket When it is consumed twice")
    @Test
    void shouldBurnTheTicketWhenItIsConsumedTwice() {
        final WsTicketService service = serviceAt(NOW);
        final String ticket = service.issue(USER);

        assertThat(service.consume(ticket)).contains(USER);
        assertThat(service.consume(ticket)).isEmpty();
    }

    @DisplayName("Should refuse an unknown ticket When it was never issued")
    @Test
    void shouldRefuseAnUnknownTicketWhenItWasNeverIssued() {
        assertThat(serviceAt(NOW).consume("made-up")).isEmpty();
        assertThat(serviceAt(NOW).consume(null)).isEmpty();
    }

    @DisplayName("Should refuse an expired ticket When the ttl has passed")
    @Test
    void shouldRefuseAnExpiredTicketWhenTheTtlHasPassed() {
        final MutableClock clock = new MutableClock(NOW);
        final WsTicketService service = new WsTicketService(properties(), clock);
        final String ticket = service.issue(USER);

        clock.advance(Duration.ofSeconds(31));

        assertThat(service.consume(ticket)).isEmpty();
    }

    @DisplayName("Should issue different tickets every time When called twice")
    @Test
    void shouldIssueDifferentTicketsEveryTimeWhenCalledTwice() {
        final WsTicketService service = serviceAt(NOW);

        assertThat(service.issue(USER)).isNotEqualTo(service.issue(USER));
    }

    private static WsTicketService serviceAt(final Instant instant) {
        return new WsTicketService(properties(), Clock.fixed(instant, ZoneOffset.UTC));
    }

    private static AuthProperties properties() {
        return new AuthProperties("test-secret-that-is-long-enough-32+", Duration.ofMinutes(15),
                Duration.ofDays(30), Duration.ofSeconds(30), List.of("invite"));
    }

    /** Часы, которые можно двигать: срок жизни тикета иначе не проверить. */
    private static final class MutableClock extends Clock {

        private Instant now;

        private MutableClock(final Instant now) {
            this.now = now;
        }

        private void advance(final Duration duration) {
            now = now.plus(duration);
        }

        @Override
        public java.time.ZoneId getZone() {
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
