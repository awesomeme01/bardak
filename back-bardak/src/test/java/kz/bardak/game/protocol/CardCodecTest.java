package kz.bardak.game.protocol;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import kz.bardak.game.rules.Card;
import kz.bardak.game.rules.DeckFactory;
import kz.bardak.game.rules.JokerCard;
import kz.bardak.game.rules.PipCard;
import kz.bardak.game.rules.Rank;
import kz.bardak.game.rules.Suit;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

/**
 * Коды карт — неизменяемый контракт: на них завязаны манифесты наборов и весь лог матчей.
 */
class CardCodecTest {

    @DisplayName("Should use the asset naming When a plain card is encoded")
    @Test
    void shouldUseTheAssetNamingWhenAPlainCardIsEncoded() {
        assertThat(CardCodec.encode(PipCard.of(Rank.SIX, Suit.DIAMONDS))).isEqualTo("6-diamonds");
        assertThat(CardCodec.encode(PipCard.of(Rank.TEN, Suit.HEARTS))).isEqualTo("10-hearts");
        assertThat(CardCodec.encode(PipCard.of(Rank.ACE, Suit.SPADES))).isEqualTo("A-spades");
    }

    @DisplayName("Should number the jokers When a joker is encoded")
    @Test
    void shouldNumberTheJokersWhenAJokerIsEncoded() {
        assertThat(CardCodec.encode(new JokerCard(3))).isEqualTo("Joker-3");
    }

    @DisplayName("Should survive the round trip When every card of the deck is encoded and decoded")
    @Test
    void shouldSurviveTheRoundTripWhenEveryCardOfTheDeckIsEncodedAndDecoded() {
        for (final Card card : new DeckFactory().buildOrdered(5)) {
            assertThat(CardCodec.decode(CardCodec.encode(card)))
                    .withFailMessage("Карта %s не пережила кодирование", card.code())
                    .isEqualTo(card);
        }
    }

    @DisplayName("Should refuse a nonsense code When it is decoded")
    @Test
    void shouldRefuseANonsenseCodeWhenItIsDecoded() {
        assertThatThrownBy(() -> CardCodec.decode("42-unicorns")).isInstanceOf(IllegalArgumentException.class);
        assertThatThrownBy(() -> CardCodec.decode("Joker-x")).isInstanceOf(IllegalArgumentException.class);
        assertThatThrownBy(() -> CardCodec.decode("")).isInstanceOf(IllegalArgumentException.class);
        assertThatThrownBy(() -> CardCodec.decode(null)).isInstanceOf(IllegalArgumentException.class);
    }
}
