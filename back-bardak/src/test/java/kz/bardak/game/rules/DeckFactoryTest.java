package kz.bardak.game.rules;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import java.util.List;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.ValueSource;

/**
 * Состав и детерминизм колоды — §1.1, §3, §6 правил.
 */
class DeckFactoryTest {

    private static final int PLAIN_CARDS = Rank.values().length * Suit.values().length;
    private static final long ANY_SEED = 20260809L;

    private final DeckFactory deckFactory = new DeckFactory();

    @DisplayName("Should hold thirty six plain cards plus one joker per player When a deck is built")
    @ParameterizedTest
    @ValueSource(ints = {2, 3, 4, 5})
    void shouldHoldThirtySixPlainCardsPlusOneJokerPerPlayerWhenADeckIsBuilt(final int playerCount) {
        final List<Card> deck = deckFactory.buildOrdered(playerCount);

        assertThat(deck).hasSize(PLAIN_CARDS + playerCount);
        assertThat(deck).filteredOn(JokerCard.class::isInstance).hasSize(playerCount);
        assertThat(deck).filteredOn(PipCard.class::isInstance).hasSize(PLAIN_CARDS);
    }

    @DisplayName("Should hold every rank and suit exactly once When a deck is built")
    @Test
    void shouldHoldEveryRankAndSuitExactlyOnceWhenADeckIsBuilt() {
        final List<Card> deck = deckFactory.buildOrdered(2);

        assertThat(deck).doesNotHaveDuplicates();
        for (final Suit suit : Suit.values()) {
            for (final Rank rank : Rank.values()) {
                assertThat(deck).contains(PipCard.of(rank, suit));
            }
        }
    }

    @DisplayName("Should number jokers from one upwards When a deck is built")
    @Test
    void shouldNumberJokersFromOneUpwardsWhenADeckIsBuilt() {
        final List<Card> deck = deckFactory.buildOrdered(5);

        assertThat(deck).filteredOn(JokerCard.class::isInstance)
                .extracting(card -> ((JokerCard) card).number())
                .containsExactly(1, 2, 3, 4, 5);
    }

    @DisplayName("Should produce the same order twice When the same seed is used")
    @Test
    void shouldProduceTheSameOrderTwiceWhenTheSameSeedIsUsed() {
        final List<Card> first = deckFactory.buildShuffled(4, ANY_SEED);
        final List<Card> second = deckFactory.buildShuffled(4, ANY_SEED);

        assertThat(first).containsExactlyElementsOf(second);
    }

    @DisplayName("Should produce a different order When the seed differs")
    @Test
    void shouldProduceADifferentOrderWhenTheSeedDiffers() {
        final List<Card> first = deckFactory.buildShuffled(4, ANY_SEED);
        final List<Card> second = deckFactory.buildShuffled(4, ANY_SEED + 1);

        assertThat(first).containsExactlyInAnyOrderElementsOf(second);
        assertThat(first).isNotEqualTo(second);
    }

    @DisplayName("Should keep the deck composition When the deck is shuffled")
    @Test
    void shouldKeepTheDeckCompositionWhenTheDeckIsShuffled() {
        final List<Card> ordered = deckFactory.buildOrdered(3);
        final List<Card> shuffled = deckFactory.buildShuffled(3, ANY_SEED);

        assertThat(shuffled).containsExactlyInAnyOrderElementsOf(ordered);
    }

    @DisplayName("Should reject a table smaller than two players When a deck is built")
    @Test
    void shouldRejectATableSmallerThanTwoPlayersWhenADeckIsBuilt() {
        assertThatThrownBy(() -> deckFactory.buildOrdered(1))
                .isInstanceOf(IllegalArgumentException.class);
    }

    @DisplayName("Should reject a table larger than five players When a deck is built")
    @Test
    void shouldRejectATableLargerThanFivePlayersWhenADeckIsBuilt() {
        assertThatThrownBy(() -> deckFactory.buildOrdered(6))
                .isInstanceOf(IllegalArgumentException.class);
    }

    @DisplayName("Should return an immutable deck When a deck is built")
    @Test
    void shouldReturnAnImmutableDeckWhenADeckIsBuilt() {
        final List<Card> deck = deckFactory.buildOrdered(2);

        assertThatThrownBy(() -> deck.add(new JokerCard(9)))
                .isInstanceOf(UnsupportedOperationException.class);
    }
}
