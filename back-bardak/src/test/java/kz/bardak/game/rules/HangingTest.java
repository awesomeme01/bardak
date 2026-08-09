package kz.bardak.game.rules;

import static kz.bardak.game.rules.DealStateFixture.aDeal;
import static org.assertj.core.api.Assertions.assertThat;

import java.util.List;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

/**
 * Навесы — центральная механика (§2.3). Окно открывается на взявшего карты, право
 * устроено тремя разными способами, а уровень поднимается ровно на одну ступень за окно.
 */
class HangingTest {

    private static final Card SEVEN_DIAMONDS = PipCard.of(Rank.SEVEN, Suit.DIAMONDS);
    private static final Card SIX_CLUBS = PipCard.of(Rank.SIX, Suit.CLUBS);
    private static final Card SIX_HEARTS = PipCard.of(Rank.SIX, Suit.HEARTS);
    private static final Card SIX_SPADES = PipCard.of(Rank.SIX, Suit.SPADES);
    private static final Card ACE_CLUBS = PipCard.of(Rank.ACE, Suit.CLUBS);
    private static final Card KING_CLUBS = PipCard.of(Rank.KING, Suit.CLUBS);

    /** Уровень «навешен туз»: следующая ступень — джокер. */
    private static final int ACE_LEVEL = NavesScale.full().jokerLevel() - 1;

    private final DealEngine engine = DealEngine.withDefaults();

    @DisplayName("Should open the window on the taker When somebody holds the flying card")
    @Test
    void shouldOpenTheWindowOnTheTakerWhenSomebodyHoldsTheFlyingCard() {
        final MoveResult result = takeAndPass(aDeal()
                .withEmptyDeck()
                .withPhase(DealPhase.DEFEND)
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(0, SIX_CLUBS)
                .withHand(1, KING_CLUBS)
                .withHand(2, ACE_CLUBS)
                .build());

        final DealState next = applied(result);
        assertThat(next.phase()).isEqualTo(DealPhase.HANGING);
        assertThat(next.hanging()).get().satisfies(window -> {
            assertThat(window.victimSeat()).isEqualTo(1);
            assertThat(window.currentStep()).containsExactly(0);
        });
        assertThat(events(result)).contains(new DealEvent.HangingWindowOpened(1));
    }

    @DisplayName("Should skip the window entirely When nobody holds the flying card")
    @Test
    void shouldSkipTheWindowEntirelyWhenNobodyHoldsTheFlyingCard() {
        final DealState next = applied(takeAndPass(aDeal()
                .withEmptyDeck()
                .withPhase(DealPhase.DEFEND)
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(0, ACE_CLUBS)
                .withHand(1, KING_CLUBS)
                .withHand(2, PipCard.of(Rank.QUEEN, Suit.CLUBS))
                .build()));

        assertThat(next.phase()).isNotEqualTo(DealPhase.HANGING);
        assertThat(next.hanging()).isEmpty();
    }

    @DisplayName("Should move the card into the victim slot and raise the level When a hang is applied")
    @Test
    void shouldMoveTheCardIntoTheVictimSlotAndRaiseTheLevelWhenAHangIsApplied() {
        final DealState window = applied(takeAndPass(aDeal()
                .withEmptyDeck()
                .withPhase(DealPhase.DEFEND)
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(0, SIX_CLUBS, ACE_CLUBS)
                .withHand(1, KING_CLUBS)
                .withHand(2, PipCard.of(Rank.QUEEN, Suit.CLUBS))
                .build()));

        final MoveResult result = engine.apply(window, new DealCommand.HangCard(0, SIX_CLUBS));

        final DealState next = applied(result);
        assertThat(next.playerAt(0).hand()).doesNotContain(SIX_CLUBS);
        assertThat(next.playerAt(1).hungCards()).containsExactly(SIX_CLUBS);
        assertThat(next.playerAt(1).navesLevel()).isZero();
        assertThat(next.hanging()).isEmpty();
        assertThat(events(result)).contains(
                new DealEvent.CardHung(0, 1, SIX_CLUBS),
                new DealEvent.NavesLevelChanged(1, 0),
                new DealEvent.HangingWindowClosed(1));
    }

    @DisplayName("Should pass the right on When the player with priority skips")
    @Test
    void shouldPassTheRightOnWhenThePlayerWithPrioritySkips() {
        final DealState window = applied(takeAndPass(aDeal()
                .withEmptyDeck()
                .withPhase(DealPhase.DEFEND)
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(0, SIX_CLUBS)
                .withHand(1, KING_CLUBS)
                .withHand(2, SIX_HEARTS)
                .withNavesLevel(0, 3)
                .withNavesLevel(2, NavesScale.NO_NAVES)
                .build()));

        assertThat(window.hanging()).get().satisfies(open ->
                assertThat(open.currentStep()).containsExactly(0));

        final DealState next = applied(engine.apply(window, new DealCommand.HangSkip(0)));

        assertThat(next.phase()).isEqualTo(DealPhase.HANGING);
        assertThat(next.hanging()).get().satisfies(open ->
                assertThat(open.currentStep()).containsExactly(2));
        assertThat(next.playerAt(1).navesLevel()).isEqualTo(NavesScale.NO_NAVES);
    }

    @DisplayName("Should close the window without a hang When everybody skipped")
    @Test
    void shouldCloseTheWindowWithoutAHangWhenEverybodySkipped() {
        final DealState window = applied(takeAndPass(aDeal()
                .withEmptyDeck()
                .withPhase(DealPhase.DEFEND)
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(0, SIX_CLUBS)
                .withHand(1, KING_CLUBS)
                .withHand(2, ACE_CLUBS)
                .withNavesLevel(0, 3)
                .withNavesLevel(2, 3)
                .build()));

        final DealState next = applied(engine.apply(window, new DealCommand.HangSkip(0)));

        assertThat(next.hanging()).isEmpty();
        assertThat(next.playerAt(1).navesLevel()).isEqualTo(NavesScale.NO_NAVES);
        assertThat(next.playerAt(1).hungCards()).isEmpty();
    }

    @DisplayName("Should refuse hanging on yourself When the victim tries to hang")
    @Test
    void shouldRefuseHangingOnYourselfWhenTheVictimTriesToHang() {
        final DealState window = applied(takeAndPass(aDeal()
                .withEmptyDeck()
                .withPhase(DealPhase.DEFEND)
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(0, SIX_CLUBS)
                .withHand(1, SIX_HEARTS)
                .withHand(2, ACE_CLUBS)
                .build()));

        assertThat(engine.apply(window, new DealCommand.HangCard(1, SIX_HEARTS)))
                .isEqualTo(MoveResult.rejected(RejectionReason.NOT_IN_HANGING_WINDOW));
    }

    @DisplayName("Should refuse a card off the scale When the rank does not fly to the victim")
    @Test
    void shouldRefuseACardOffTheScaleWhenTheRankDoesNotFlyToTheVictim() {
        final DealState window = applied(takeAndPass(aDeal()
                .withEmptyDeck()
                .withPhase(DealPhase.DEFEND)
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(0, SIX_CLUBS, ACE_CLUBS)
                .withHand(1, KING_CLUBS)
                .withHand(2, PipCard.of(Rank.QUEEN, Suit.CLUBS))
                .build()));

        assertThat(engine.apply(window, new DealCommand.HangCard(0, ACE_CLUBS)))
                .isEqualTo(MoveResult.rejected(RejectionReason.CARD_NOT_ON_NAVES_SCALE));
    }

    @DisplayName("Should give the right to everyone at once When the unique laggard is the victim")
    @Test
    void shouldGiveTheRightToEveryoneAtOnceWhenTheUniqueLaggardIsTheVictim() {
        final DealState window = applied(takeAndPass(aDeal()
                .withEmptyDeck()
                .withPhase(DealPhase.DEFEND)
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(0, SIX_CLUBS)
                .withHand(1, KING_CLUBS)
                .withHand(2, SIX_HEARTS)
                .withNavesLevel(0, 3)
                .withNavesLevel(2, 5)
                .build()));

        assertThat(window.hanging()).get().satisfies(open -> {
            assertThat(open.currentStep()).containsExactlyInAnyOrder(0, 2);
            assertThat(open.everyClaimantHangs()).isTrue();
        });
    }

    @DisplayName("Should hang every claimed card but raise the level once When the whole table finishes the laggard")
    @Test
    void shouldHangEveryClaimedCardButRaiseTheLevelOnceWhenTheWholeTableFinishesTheLaggard() {
        final DealState window = applied(takeAndPass(aDeal()
                .withEmptyDeck()
                .withPhase(DealPhase.DEFEND)
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(0, SIX_CLUBS)
                .withHand(1, KING_CLUBS)
                .withHand(2, SIX_HEARTS)
                .withNavesLevel(0, 3)
                .withNavesLevel(2, 5)
                .build()));

        final DealState afterFirst = applied(engine.apply(window, new DealCommand.HangCard(0, SIX_CLUBS)));
        final DealState next = applied(engine.apply(afterFirst, new DealCommand.HangCard(2, SIX_HEARTS)));

        assertThat(next.playerAt(1).hungCards()).containsExactlyInAnyOrder(SIX_CLUBS, SIX_HEARTS);
        assertThat(next.playerAt(1).navesLevel()).isZero();
        assertThat(next.playerAt(0).hand()).doesNotContain(SIX_CLUBS);
        assertThat(next.playerAt(2).hand()).doesNotContain(SIX_HEARTS);
    }

    @DisplayName("Should fall back to the priority queue When the lowest level is shared")
    @Test
    void shouldFallBackToThePriorityQueueWhenTheLowestLevelIsShared() {
        final DealState window = applied(takeAndPass(aDeal()
                .withEmptyDeck()
                .withPhase(DealPhase.DEFEND)
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(0, SIX_CLUBS)
                .withHand(1, KING_CLUBS)
                .withHand(2, SIX_HEARTS)
                .withNavesLevel(2, NavesScale.NO_NAVES)
                .withNavesLevel(0, 3)
                .build()));

        assertThat(window.hanging()).get().satisfies(open -> {
            assertThat(open.currentStep()).containsExactly(0);
            assertThat(open.everyClaimantHangs()).isFalse();
        });
    }

    @DisplayName("Should ignore players out of the deal When the laggard is looked for")
    @Test
    void shouldIgnorePlayersOutOfTheDealWhenTheLaggardIsLookedFor() {
        final HangingRules rules = new HangingRules(RulesConfig.defaults());
        final DealState state = aDeal()
                .withNavesLevel(0, 4)
                .withNavesLevel(1, 2)
                .withNavesLevel(2, NavesScale.NO_NAVES)
                .withPlayerOutOfDeal(2)
                .build();

        assertThat(rules.isUniqueLaggard(state, 1)).isTrue();
    }

    @DisplayName("Should give the joker right to everyone at once When the victim stands on the ace")
    @Test
    void shouldGiveTheJokerRightToEveryoneAtOnceWhenTheVictimStandsOnTheAce() {
        final DealState window = applied(takeAndPass(aDeal()
                .withEmptyDeck()
                .withPhase(DealPhase.DEFEND)
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(0, new JokerCard(1))
                .withHand(1, KING_CLUBS)
                .withHand(2, new JokerCard(2))
                .withNavesLevel(1, ACE_LEVEL)
                .build()));

        assertThat(window.hanging()).get().satisfies(open -> {
            assertThat(open.currentStep()).containsExactlyInAnyOrder(0, 2);
            assertThat(open.everyClaimantHangs()).isFalse();
        });
    }

    @DisplayName("Should let the dice pick a single winner When several claim the joker")
    @Test
    void shouldLetTheDicePickASingleWinnerWhenSeveralClaimTheJoker() {
        final DiceResolver alwaysLastSeat = (seats, seed, rollNo) -> seats.get(seats.size() - 1);
        final DealEngine riggedEngine = new DealEngine(RulesConfig.defaults(),
                new AttackOrderPolicy.BardakStrictNeighbours(), alwaysLastSeat);
        final DealState window = applied(takeAndPass(riggedEngine, aDeal()
                .withEmptyDeck()
                .withPhase(DealPhase.DEFEND)
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(0, new JokerCard(1))
                .withHand(1, KING_CLUBS)
                .withHand(2, new JokerCard(2))
                .withNavesLevel(1, ACE_LEVEL)
                .build()));

        final DealState afterFirst = applied(
                riggedEngine.apply(window, new DealCommand.HangCard(0, new JokerCard(1))));
        final MoveResult result = riggedEngine.apply(afterFirst, new DealCommand.HangCard(2, new JokerCard(2)));

        final DealState next = applied(result);
        assertThat(next.playerAt(1).hungCards()).containsExactly(new JokerCard(2));
        assertThat(next.playerAt(2).hand()).doesNotContain(new JokerCard(2));
        assertThat(next.playerAt(0).hand()).contains(new JokerCard(1));
        assertThat(events(result)).contains(new DealEvent.DiceRolled(2, List.of(0, 2)));
    }

    @DisplayName("Should never open the window When naves are switched off in the table config")
    @Test
    void shouldNeverOpenTheWindowWhenNavesAreSwitchedOffInTheTableConfig() {
        final DealEngine plainEngine = DealEngine.of(RulesConfig.defaults().withoutNaves());
        final DealState next = applied(takeAndPass(plainEngine, aDeal()
                .withEmptyDeck()
                .withPhase(DealPhase.DEFEND)
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(0, SIX_CLUBS)
                .withHand(1, KING_CLUBS)
                .withHand(2, SIX_SPADES)
                .build()));

        assertThat(next.phase()).isNotEqualTo(DealPhase.HANGING);
        assertThat(next.playerAt(1).navesLevel()).isEqualTo(NavesScale.NO_NAVES);
    }

    private MoveResult takeAndPass(final DealState state) {
        return takeAndPass(engine, state);
    }

    /** «Беру» → подкидывающие пасуют → стол уехал в руку → открывается окно навеса. */
    private static MoveResult takeAndPass(final DealEngine engine, final DealState state) {
        final DealState announced = applied(engine.apply(state, new DealCommand.Take(1)));
        final DealState afterFirstPass = applied(engine.apply(announced, new DealCommand.Pass(0)));
        return engine.apply(afterFirstPass, new DealCommand.Pass(2));
    }

    private static DealState applied(final MoveResult result) {
        assertThat(result).isInstanceOf(MoveResult.Applied.class);
        return ((MoveResult.Applied) result).state();
    }

    private static List<DealEvent> events(final MoveResult result) {
        return ((MoveResult.Applied) result).events();
    }
}
