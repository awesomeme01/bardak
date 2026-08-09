package kz.bardak.game.rules;

import static org.assertj.core.api.Assertions.assertThat;

import java.util.List;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

/**
 * Личная шкала навесов — §2.3, ADR-017. Счёта в очках нет: шкала и есть счёт.
 */
class NavesScaleTest {

    private final NavesScale scale = NavesScale.full();

    @DisplayName("Should send the lowest rank first When nothing has been hung yet")
    @Test
    void shouldSendTheLowestRankFirstWhenNothingHasBeenHungYet() {
        assertThat(scale.nextRank(NavesScale.NO_NAVES)).contains(Rank.SIX);
    }

    @DisplayName("Should send the next rank up When a card has been hung")
    @Test
    void shouldSendTheNextRankUpWhenACardHasBeenHung() {
        assertThat(scale.nextRank(0)).contains(Rank.SEVEN);
        assertThat(scale.nextRank(7)).contains(Rank.ACE);
    }

    @DisplayName("Should send a joker after the top rank When the scale is exhausted")
    @Test
    void shouldSendAJokerAfterTheTopRankWhenTheScaleIsExhausted() {
        final int aceLevel = scale.jokerLevel() - 1;

        assertThat(scale.nextIsJoker(aceLevel)).isTrue();
        assertThat(scale.nextRank(aceLevel)).isEmpty();
        assertThat(scale.isFlyingCard(aceLevel, new JokerCard(1))).isTrue();
        assertThat(scale.isFlyingCard(aceLevel, PipCard.of(Rank.ACE, Suit.SPADES))).isFalse();
    }

    @DisplayName("Should accept the flying rank in any suit When a card is checked")
    @Test
    void shouldAcceptTheFlyingRankInAnySuitWhenACardIsChecked() {
        assertThat(scale.isFlyingCard(NavesScale.NO_NAVES, PipCard.of(Rank.SIX, Suit.CLUBS))).isTrue();
        assertThat(scale.isFlyingCard(NavesScale.NO_NAVES, PipCard.of(Rank.SIX, Suit.HEARTS))).isTrue();
        assertThat(scale.isFlyingCard(NavesScale.NO_NAVES, PipCard.of(Rank.SEVEN, Suit.CLUBS))).isFalse();
    }

    @DisplayName("Should refuse any card When the joker has already been hung")
    @Test
    void shouldRefuseAnyCardWhenTheJokerHasAlreadyBeenHung() {
        final int finished = scale.jokerLevel();

        assertThat(scale.isFinished(finished)).isTrue();
        assertThat(scale.isFlyingCard(finished, new JokerCard(1))).isFalse();
    }

    @DisplayName("Should follow the configured length When the scale is shortened")
    @Test
    void shouldFollowTheConfiguredLengthWhenTheScaleIsShortened() {
        final NavesScale shortened = new NavesScale(List.of(Rank.NINE, Rank.TEN, Rank.JACK));

        assertThat(shortened.nextRank(NavesScale.NO_NAVES)).contains(Rank.NINE);
        assertThat(shortened.nextIsJoker(2)).isTrue();
        assertThat(shortened.jokerLevel()).isEqualTo(3);
    }
}
