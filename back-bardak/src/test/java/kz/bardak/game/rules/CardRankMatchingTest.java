package kz.bardak.game.rules;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.EnumSource;

/**
 * Совпадение по рангу — то, что разрешает подкинуть и перевести (§2.1, §2.2, ADR-032).
 */
class CardRankMatchingTest {

    @DisplayName("Should match a plain card of the same rank When suits differ")
    @Test
    void shouldMatchAPlainCardOfTheSameRankWhenSuitsDiffer() {
        final Card seven = PipCard.of(Rank.SEVEN, Suit.DIAMONDS);

        assertThat(seven.sameRankAs(PipCard.of(Rank.SEVEN, Suit.CLUBS))).isTrue();
    }

    @DisplayName("Should not match a plain card of another rank When the suit is the same")
    @Test
    void shouldNotMatchAPlainCardOfAnotherRankWhenTheSuitIsTheSame() {
        final Card seven = PipCard.of(Rank.SEVEN, Suit.DIAMONDS);

        assertThat(seven.sameRankAs(PipCard.of(Rank.EIGHT, Suit.DIAMONDS))).isFalse();
    }

    @DisplayName("Should match a joker only with another joker When the rank of a joker is checked")
    @Test
    void shouldMatchAJokerOnlyWithAnotherJokerWhenTheRankOfAJokerIsChecked() {
        final Card joker = new JokerCard(1);

        assertThat(joker.sameRankAs(new JokerCard(2))).isTrue();
        assertThat(joker.sameRankAs(PipCard.of(Rank.ACE, Suit.SPADES))).isFalse();
    }

    @DisplayName("Should not match a joker with any plain rank When a plain card is checked")
    @ParameterizedTest
    @EnumSource(Rank.class)
    void shouldNotMatchAJokerWithAnyPlainRankWhenAPlainCardIsChecked(final Rank rank) {
        final Card plain = PipCard.of(rank, Suit.HEARTS);

        assertThat(plain.sameRankAs(new JokerCard(1))).isFalse();
    }

    @DisplayName("Should stay symmetric When two cards are compared in both directions")
    @Test
    void shouldStaySymmetricWhenTwoCardsAreComparedInBothDirections() {
        final Card jack = PipCard.of(Rank.JACK, Suit.SPADES);
        final Card joker = new JokerCard(4);

        assertThat(jack.sameRankAs(joker)).isEqualTo(joker.sameRankAs(jack));
    }

    @DisplayName("Should reject a joker number below one When a joker is created")
    @Test
    void shouldRejectAJokerNumberBelowOneWhenAJokerIsCreated() {
        assertThatThrownBy(() -> new JokerCard(0))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("0");
    }

    @DisplayName("Should render a readable code When a card is printed")
    @Test
    void shouldRenderAReadableCodeWhenACardIsPrinted() {
        assertThat(PipCard.of(Rank.TEN, Suit.HEARTS).code()).isEqualTo("10♥");
        assertThat(new JokerCard(3).code()).isEqualTo("Joker-3");
    }
}
