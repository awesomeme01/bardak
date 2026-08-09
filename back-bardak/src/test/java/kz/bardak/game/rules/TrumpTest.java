package kz.bardak.game.rules;

import static org.assertj.core.api.Assertions.assertThat;

import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.EnumSource;

/**
 * Старшинство карт и защищённая масть — §1.1, §1.1.1, §3.1 правил.
 */
class TrumpTest {

    private static final Trump HEARTS_TRUMP = Trump.of(Suit.HEARTS);

    @DisplayName("Should beat lower card of the same suit When ranks are compared inside one suit")
    @Test
    void shouldBeatLowerCardOfTheSameSuitWhenRanksAreComparedInsideOneSuit() {
        final Card attack = PipCard.of(Rank.SEVEN, Suit.DIAMONDS);
        final Card defence = PipCard.of(Rank.NINE, Suit.DIAMONDS);

        assertThat(HEARTS_TRUMP.beats(defence, attack)).isTrue();
    }

    @DisplayName("Should not beat higher card of the same suit When the defence rank is lower")
    @Test
    void shouldNotBeatHigherCardOfTheSameSuitWhenTheDefenceRankIsLower() {
        final Card attack = PipCard.of(Rank.NINE, Suit.DIAMONDS);
        final Card defence = PipCard.of(Rank.SEVEN, Suit.DIAMONDS);

        assertThat(HEARTS_TRUMP.beats(defence, attack)).isFalse();
    }

    @DisplayName("Should not beat equal rank of another suit When neither card is trump")
    @Test
    void shouldNotBeatEqualRankOfAnotherSuitWhenNeitherCardIsTrump() {
        final Card attack = PipCard.of(Rank.NINE, Suit.DIAMONDS);
        final Card defence = PipCard.of(Rank.KING, Suit.CLUBS);

        assertThat(HEARTS_TRUMP.beats(defence, attack)).isFalse();
    }

    @DisplayName("Should beat plain card with the lowest trump When suits differ")
    @Test
    void shouldBeatPlainCardWithTheLowestTrumpWhenSuitsDiffer() {
        final Card attack = PipCard.of(Rank.ACE, Suit.DIAMONDS);
        final Card defence = PipCard.of(Rank.SIX, Suit.HEARTS);

        assertThat(HEARTS_TRUMP.beats(defence, attack)).isTrue();
    }

    @DisplayName("Should not beat trump with a plain card When the defence is not trump")
    @Test
    void shouldNotBeatTrumpWithAPlainCardWhenTheDefenceIsNotTrump() {
        final Card attack = PipCard.of(Rank.SIX, Suit.HEARTS);
        final Card defence = PipCard.of(Rank.ACE, Suit.DIAMONDS);

        assertThat(HEARTS_TRUMP.beats(defence, attack)).isFalse();
    }

    @DisplayName("Should protect spades from trump When the trump suit is not spades")
    @Test
    void shouldProtectSpadesFromTrumpWhenTheTrumpSuitIsNotSpades() {
        final Card attack = PipCard.of(Rank.SIX, Suit.SPADES);
        final Card trumpAce = PipCard.of(Rank.ACE, Suit.HEARTS);

        assertThat(HEARTS_TRUMP.protectedSuit()).isEqualTo(Suit.SPADES);
        assertThat(HEARTS_TRUMP.beats(trumpAce, attack)).isFalse();
    }

    @DisplayName("Should beat a spade only with a higher spade When spades are the protected suit")
    @Test
    void shouldBeatASpadeOnlyWithAHigherSpadeWhenSpadesAreTheProtectedSuit() {
        final Card attack = PipCard.of(Rank.TEN, Suit.SPADES);

        assertThat(HEARTS_TRUMP.beats(PipCard.of(Rank.JACK, Suit.SPADES), attack)).isTrue();
        assertThat(HEARTS_TRUMP.beats(PipCard.of(Rank.NINE, Suit.SPADES), attack)).isFalse();
    }

    @DisplayName("Should move the protected suit to clubs When the trump suit is spades")
    @Test
    void shouldMoveTheProtectedSuitToClubsWhenTheTrumpSuitIsSpades() {
        final Trump spadesTrump = Trump.of(Suit.SPADES);
        final Card clubAttack = PipCard.of(Rank.SIX, Suit.CLUBS);
        final Card spadeAce = PipCard.of(Rank.ACE, Suit.SPADES);

        assertThat(spadesTrump.protectedSuit()).isEqualTo(Suit.CLUBS);
        assertThat(spadesTrump.beats(spadeAce, clubAttack)).isFalse();
        assertThat(spadesTrump.beats(PipCard.of(Rank.SEVEN, Suit.CLUBS), clubAttack)).isTrue();
    }

    @DisplayName("Should stop protecting spades When spades became the trump suit")
    @Test
    void shouldStopProtectingSpadesWhenSpadesBecameTheTrumpSuit() {
        final Trump spadesTrump = Trump.of(Suit.SPADES);
        final Card diamondAttack = PipCard.of(Rank.ACE, Suit.DIAMONDS);

        assertThat(spadesTrump.beats(PipCard.of(Rank.SIX, Suit.SPADES), diamondAttack)).isTrue();
    }

    @DisplayName("Should always have exactly one protected suit When any suit is trump")
    @ParameterizedTest
    @EnumSource(Suit.class)
    void shouldAlwaysHaveExactlyOneProtectedSuitWhenAnySuitIsTrump(final Suit suit) {
        final Trump trump = Trump.of(suit);

        assertThat(trump.protectedSuit()).isNotNull().isNotEqualTo(suit);
    }

    @DisplayName("Should beat any card with a joker When the joker defends")
    @Test
    void shouldBeatAnyCardWithAJokerWhenTheJokerDefends() {
        final Card joker = new JokerCard(1);

        assertThat(HEARTS_TRUMP.beats(joker, PipCard.of(Rank.ACE, Suit.HEARTS))).isTrue();
        assertThat(HEARTS_TRUMP.beats(joker, PipCard.of(Rank.ACE, Suit.SPADES))).isTrue();
        assertThat(HEARTS_TRUMP.beats(joker, new JokerCard(2))).isTrue();
    }

    @DisplayName("Should not beat a joker with any plain card When the attack is a joker")
    @Test
    void shouldNotBeatAJokerWithAnyPlainCardWhenTheAttackIsAJoker() {
        final Card jokerAttack = new JokerCard(3);

        assertThat(HEARTS_TRUMP.beats(PipCard.of(Rank.ACE, Suit.HEARTS), jokerAttack)).isFalse();
        assertThat(HEARTS_TRUMP.beats(PipCard.of(Rank.ACE, Suit.SPADES), jokerAttack)).isFalse();
    }
}
