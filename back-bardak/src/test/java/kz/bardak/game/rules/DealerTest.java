package kz.bardak.game.rules;

import static org.assertj.core.api.Assertions.assertThat;

import java.util.ArrayList;
import java.util.List;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.ValueSource;

/**
 * Сдача раздачи — §1.2, §1.8, §6.
 */
class DealerTest {

    private static final RulesConfig CONFIG = RulesConfig.defaults();

    private final Dealer dealer = new Dealer(CONFIG, new DiceResolver.Seeded());

    @DisplayName("Should give every player a hand and one face-down card When a deal starts")
    @ParameterizedTest
    @ValueSource(ints = {2, 3, 4, 5})
    void shouldGiveEveryPlayerAHandAndOneFaceDownCardWhenADealStarts(final int playerCount) {
        final DealState deal = dealer.startDeal(freshLevels(playerCount), 7L);

        assertThat(deal.players()).hasSize(playerCount);
        assertThat(deal.players()).allSatisfy(player -> {
            assertThat(player.handSize()).isEqualTo(CONFIG.dealSize());
            assertThat(player.hasFaceDownCard()).isTrue();
            assertThat(player.inDeal()).isTrue();
        });
    }

    @DisplayName("Should leave the rest of the deck for drawing When a deal starts")
    @Test
    void shouldLeaveTheRestOfTheDeckForDrawingWhenADealStarts() {
        final int playerCount = 4;
        final DealState deal = dealer.startDeal(freshLevels(playerCount), 7L);
        final int dealt = playerCount * (CONFIG.dealSize() + 1);

        assertThat(deal.deck()).hasSize(new DeckFactory().buildOrdered(playerCount).size() - dealt);
    }

    @DisplayName("Should hand out every card exactly once When a deal starts")
    @Test
    void shouldHandOutEveryCardExactlyOnceWhenADealStarts() {
        final DealState deal = dealer.startDeal(freshLevels(3), 11L);

        final List<Card> everywhere = new ArrayList<>(deal.deck());
        for (final PlayerState player : deal.players()) {
            everywhere.addAll(player.hand());
            player.faceDown().ifPresent(everywhere::add);
        }

        assertThat(everywhere).doesNotHaveDuplicates()
                .containsExactlyInAnyOrderElementsOf(new DeckFactory().buildOrdered(3));
    }

    @DisplayName("Should carry the naves levels into the deal When levels come from the match")
    @Test
    void shouldCarryTheNavesLevelsIntoTheDealWhenLevelsComeFromTheMatch() {
        final DealState deal = dealer.startDeal(List.of(0, 4, NavesScale.NO_NAVES), 7L);

        assertThat(deal.players()).extracting(PlayerState::navesLevel)
                .containsExactly(0, 4, NavesScale.NO_NAVES);
        assertThat(deal.players()).allSatisfy(player -> assertThat(player.hungCards()).isEmpty());
    }

    @DisplayName("Should deal the same cards twice When the same seed is used")
    @Test
    void shouldDealTheSameCardsTwiceWhenTheSameSeedIsUsed() {
        final DealState first = dealer.startDeal(freshLevels(4), 20260810L);
        final DealState second = dealer.startDeal(freshLevels(4), 20260810L);

        assertThat(first.players()).isEqualTo(second.players());
        assertThat(first.deck()).isEqualTo(second.deck());
        assertThat(first.trump()).isEqualTo(second.trump());
    }

    @DisplayName("Should give the first move to the lowest trump When the trump is known")
    @Test
    void shouldGiveTheFirstMoveToTheLowestTrumpWhenTheTrumpIsKnown() {
        final DealState deal = dealFinishedByCard();

        final Rank starterLowest = lowestTrumpRank(deal, deal.roundStarterSeat());
        for (final PlayerState player : deal.players()) {
            final Rank candidate = lowestTrumpRank(deal, player.seatNo());
            if (candidate != null) {
                assertThat(starterLowest).isNotNull();
                assertThat(candidate.isHigherThan(starterLowest) || candidate == starterLowest).isTrue();
            }
        }
        assertThat(deal.defenderSeat()).isEqualTo((deal.roundStarterSeat() + 1) % deal.players().size());
    }

    @DisplayName("Should always leave a trump in somebody's hand When a deal is dealt")
    @ParameterizedTest
    @ValueSource(ints = {2, 3, 4, 5})
    void shouldAlwaysLeaveATrumpInSomebodysHandWhenADealIsDealt(final int playerCount) {
        for (long seed = 1; seed <= 60; seed++) {
            final DealState deal = dealer.startDeal(freshLevels(playerCount), seed);
            if (deal.hasTrump()) {
                assertThat(dealer.hasAnyTrumpInHands(deal))
                        .withFailMessage("seed %d: козыря нет ни у кого, раздача не пересдалась", seed)
                        .isTrue();
            }
        }
    }

    @DisplayName("Should reshuffle deterministically When the same seed is dealt twice")
    @Test
    void shouldReshuffleDeterministicallyWhenTheSameSeedIsDealtTwice() {
        final long seed = 3L;

        assertThat(dealer.startDeal(freshLevels(2), seed).players())
                .isEqualTo(dealer.startDeal(freshLevels(2), seed).players());
    }

    @DisplayName("Should open the dice phase When the trump card is a joker")
    @Test
    void shouldOpenTheDicePhaseWhenTheTrumpCardIsAJoker() {
        final DealState deal = dealWithJokerAtTheBottom();

        assertThat(deal.phase()).isEqualTo(DealPhase.DICE);
        assertThat(deal.hasTrump()).isFalse();
        assertThat(deal.deck().get(deal.deck().size() - 2)).isInstanceOf(JokerCard.class);
    }

    @DisplayName("Should keep the hidden trump under the trump card When a deal starts")
    @Test
    void shouldKeepTheHiddenTrumpUnderTheTrumpCardWhenADealStarts() {
        final DealState deal = dealFinishedByCard();
        final List<Card> deck = deal.deck();

        assertThat(deck).hasSizeGreaterThan(1);
        assertThat(deck.get(deck.size() - 2))
                .isInstanceOfSatisfying(PipCard.class,
                        card -> assertThat(card.suit()).isEqualTo(deal.trump().suit()));
        assertThat(deal.hiddenTrumpAwaitingSuit()).isEmpty();
    }

    private static Rank lowestTrumpRank(final DealState deal, final int seatNo) {
        return deal.playerAt(seatNo).hand().stream()
                .filter(PipCard.class::isInstance)
                .map(PipCard.class::cast)
                .filter(card -> card.suit() == deal.trump().suit())
                .map(PipCard::rank)
                .min(java.util.Comparator.naturalOrder())
                .orElse(null);
    }

    /** Первый seed, при котором нижней картой оказывается обычная карта. */
    private DealState dealFinishedByCard() {
        return firstDealMatching(false);
    }

    /** Первый seed, при котором нижней картой оказывается джокер. */
    private DealState dealWithJokerAtTheBottom() {
        return firstDealMatching(true);
    }

    private DealState firstDealMatching(final boolean jokerAtTheBottom) {
        for (long seed = 1; seed < 200; seed++) {
            final DealState deal = dealer.startDeal(freshLevels(4), seed);
            if (deal.hasTrump() != jokerAtTheBottom) {
                return deal;
            }
        }
        throw new IllegalStateException("Не нашлось seed с нужной нижней картой");
    }

    private static List<Integer> freshLevels(final int playerCount) {
        final List<Integer> levels = new ArrayList<>();
        for (int seat = 0; seat < playerCount; seat++) {
            levels.add(NavesScale.NO_NAVES);
        }
        return List.copyOf(levels);
    }
}
