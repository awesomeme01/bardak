package kz.bardak.game.rules;

import static kz.bardak.game.rules.DealStateFixture.aDeal;
import static org.assertj.core.api.Assertions.assertThat;

import java.util.List;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

/**
 * Переходы автомата раздачи — §1.4, §2.1, §2.2, §4.2 правил.
 */
class DealEngineTest {

    private static final Card SEVEN_DIAMONDS = PipCard.of(Rank.SEVEN, Suit.DIAMONDS);
    private static final Card SEVEN_CLUBS = PipCard.of(Rank.SEVEN, Suit.CLUBS);
    private static final Card SEVEN_SPADES = PipCard.of(Rank.SEVEN, Suit.SPADES);
    private static final Card NINE_DIAMONDS = PipCard.of(Rank.NINE, Suit.DIAMONDS);
    private static final Card ACE_CLUBS = PipCard.of(Rank.ACE, Suit.CLUBS);
    private static final Card KING_CLUBS = PipCard.of(Rank.KING, Suit.CLUBS);

    private final DealEngine engine = DealEngine.withDefaults();

    @DisplayName("Should move the card from hand to the table When an attack is applied")
    @Test
    void shouldMoveTheCardFromHandToTheTableWhenAnAttackIsApplied() {
        final DealState state = aDeal().withHand(0, SEVEN_DIAMONDS, ACE_CLUBS).withHand(1, KING_CLUBS).build();

        final MoveResult result = engine.apply(state, new DealCommand.Attack(0, SEVEN_DIAMONDS));

        final DealState next = applied(result);
        assertThat(next.playerAt(0).hand()).containsExactly(ACE_CLUBS);
        assertThat(next.table()).singleElement().satisfies(slot -> {
            assertThat(slot.attack()).isEqualTo(SEVEN_DIAMONDS);
            assertThat(slot.isBeaten()).isFalse();
        });
        assertThat(next.phase()).isEqualTo(DealPhase.DEFEND);
        assertThat(events(result)).containsExactly(new DealEvent.CardAttacked(0, SEVEN_DIAMONDS));
    }

    @DisplayName("Should leave the state untouched When a command is rejected")
    @Test
    void shouldLeaveTheStateUntouchedWhenACommandIsRejected() {
        final DealState state = aDeal().withHand(2, SEVEN_DIAMONDS).withHand(1, KING_CLUBS).build();

        final MoveResult result = engine.apply(state, new DealCommand.Attack(2, SEVEN_DIAMONDS));

        assertThat(result).isEqualTo(MoveResult.rejected(RejectionReason.NOT_YOUR_TURN));
    }

    @DisplayName("Should mark the named slot as beaten When a defence is applied")
    @Test
    void shouldMarkTheNamedSlotAsBeatenWhenADefenceIsApplied() {
        final DealState state = aDeal()
                .withPhase(DealPhase.DEFEND)
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(1, NINE_DIAMONDS)
                .build();

        final MoveResult result = engine.apply(state, new DealCommand.Defend(1, NINE_DIAMONDS, SEVEN_DIAMONDS));

        final DealState next = applied(result);
        assertThat(next.table()).singleElement().satisfies(slot ->
                assertThat(slot.defenceCard()).contains(NINE_DIAMONDS));
        assertThat(next.anyCardBeatenThisRound()).isTrue();
        assertThat(next.phase()).isEqualTo(DealPhase.ATTACK);
        assertThat(next.playerAt(1).hand()).isEmpty();
    }

    @DisplayName("Should stay in defence When unbeaten cards remain on the table")
    @Test
    void shouldStayInDefenceWhenUnbeatenCardsRemainOnTheTable() {
        final DealState state = aDeal()
                .withPhase(DealPhase.DEFEND)
                .withAttackCards(SEVEN_DIAMONDS, SEVEN_CLUBS)
                .withHand(1, NINE_DIAMONDS, PipCard.of(Rank.NINE, Suit.CLUBS))
                .build();

        final MoveResult result = engine.apply(state, new DealCommand.Defend(1, NINE_DIAMONDS, SEVEN_DIAMONDS));

        assertThat(applied(result).phase()).isEqualTo(DealPhase.DEFEND);
    }

    @DisplayName("Should make the transferring player the attacker When an attack is transferred")
    @Test
    void shouldMakeTheTransferringPlayerTheAttackerWhenAnAttackIsTransferred() {
        final DealState state = aDeal()
                .withPhase(DealPhase.DEFEND)
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(1, SEVEN_CLUBS)
                .withHand(2, ACE_CLUBS, KING_CLUBS)
                .build();

        final MoveResult result = engine.apply(state, new DealCommand.Transfer(1, SEVEN_CLUBS));

        final DealState next = applied(result);
        assertThat(next.defenderSeat()).isEqualTo(2);
        assertThat(next.roundStarterSeat()).isEqualTo(1);
        assertThat(next.attackRightSeat()).isEqualTo(1);
        assertThat(next.attackCardCount()).isEqualTo(2);
        assertThat(events(result)).containsExactly(new DealEvent.AttackTransferred(1, 2, SEVEN_CLUBS));
    }

    @DisplayName("Should hand the attack right to the second neighbour When the round starter passes")
    @Test
    void shouldHandTheAttackRightToTheSecondNeighbourWhenTheRoundStarterPasses() {
        final DealState state = aDeal()
                .withSeats(4)
                .withPhase(DealPhase.DEFEND)
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(1, NINE_DIAMONDS)
                .withHand(2, SEVEN_CLUBS)
                .build();

        final MoveResult result = engine.apply(state, new DealCommand.Pass(0));

        final DealState next = applied(result);
        assertThat(next.attackRightSeat()).isEqualTo(2);
        assertThat(next.hasPassed(0)).isTrue();
        assertThat(events(result)).containsExactly(new DealEvent.Passed(0), new DealEvent.AttackRightMoved(2));
    }

    @DisplayName("Should never give the attack right back When the passed player tries to attack again")
    @Test
    void shouldNeverGiveTheAttackRightBackWhenThePassedPlayerTriesToAttackAgain() {
        final DealState state = aDeal()
                .withSeats(4)
                .withAttackCards(SEVEN_DIAMONDS)
                .withPassed(0)
                .withHand(0, SEVEN_CLUBS)
                .withHand(1, NINE_DIAMONDS, ACE_CLUBS)
                .build();

        assertThat(engine.apply(state, new DealCommand.Attack(0, SEVEN_CLUBS)))
                .isEqualTo(MoveResult.rejected(RejectionReason.NOT_YOUR_TURN));
    }

    @DisplayName("Should close the round as beaten When the last attacker passes and nothing is unbeaten")
    @Test
    void shouldCloseTheRoundAsBeatenWhenTheLastAttackerPassesAndNothingIsUnbeaten() {
        final DealState state = aDeal()
                .withEmptyDeck()
                .withBeatenPair(SEVEN_DIAMONDS, NINE_DIAMONDS)
                .withHand(0, ACE_CLUBS)
                .withHand(1, KING_CLUBS)
                .withHand(2, SEVEN_CLUBS)
                .build();

        final DealState afterFirstPass = applied(engine.apply(state, new DealCommand.Pass(0)));
        assertThat(afterFirstPass.attackRightSeat()).isEqualTo(2);
        assertThat(afterFirstPass.table()).hasSize(1);

        final MoveResult result = engine.apply(afterFirstPass, new DealCommand.Pass(2));

        final DealState next = applied(result);
        assertThat(next.table()).isEmpty();
        assertThat(next.anyPileDiscarded()).isTrue();
        assertThat(next.roundStarterSeat()).isEqualTo(1);
        assertThat(next.defenderSeat()).isEqualTo(2);
        assertThat(next.phase()).isEqualTo(DealPhase.ATTACK);
        assertThat(events(result)).contains(new DealEvent.RoundBeaten(1, List.of(SEVEN_DIAMONDS, NINE_DIAMONDS)));
    }

    @DisplayName("Should wait for the defender When every attacker passed but cards are unbeaten")
    @Test
    void shouldWaitForTheDefenderWhenEveryAttackerPassedButCardsAreUnbeaten() {
        final DealState state = aDeal()
                .withPhase(DealPhase.DEFEND)
                .withAttackCards(SEVEN_DIAMONDS)
                .withPassed(2)
                .withHand(1, ACE_CLUBS)
                .build();

        final MoveResult result = engine.apply(state, new DealCommand.Pass(0));

        final DealState next = applied(result);
        assertThat(next.phase()).isEqualTo(DealPhase.DEFEND);
        assertThat(next.table()).hasSize(1);
    }

    @DisplayName("Should keep the round alive for follow-up cards When the defender announces a take")
    @Test
    void shouldKeepTheRoundAliveForFollowUpCardsWhenTheDefenderAnnouncesATake() {
        final DealState state = aDeal()
                .withEmptyDeck()
                .withPhase(DealPhase.DEFEND)
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(0, SEVEN_CLUBS)
                .withHand(1, KING_CLUBS)
                .withHand(2, SEVEN_SPADES)
                .build();

        final MoveResult result = engine.apply(state, new DealCommand.Take(1));

        final DealState next = applied(result);
        assertThat(next.phase()).isEqualTo(DealPhase.TAKING);
        assertThat(next.table()).hasSize(1);
        assertThat(next.playerAt(1).hand()).containsExactly(KING_CLUBS);
        assertThat(events(result)).containsExactly(new DealEvent.TakeAnnounced(1));
    }

    @DisplayName("Should move the whole table into the hand When every attacker passed after the take")
    @Test
    void shouldMoveTheWholeTableIntoTheHandWhenEveryAttackerPassedAfterTheTake() {
        final DealState state = aDeal()
                .withEmptyDeck()
                .withPhase(DealPhase.DEFEND)
                .withBeatenPair(SEVEN_DIAMONDS, NINE_DIAMONDS)
                .withAttackCards(SEVEN_CLUBS)
                .withHand(0, ACE_CLUBS)
                .withHand(1, KING_CLUBS)
                .withHand(2, SEVEN_SPADES)
                .build();

        final DealState announced = applied(engine.apply(state, new DealCommand.Take(1)));
        final DealState afterFirstPass = applied(engine.apply(announced, new DealCommand.Pass(0)));
        final MoveResult result = engine.apply(afterFirstPass, new DealCommand.Pass(2));

        final DealState next = applied(result);
        assertThat(next.playerAt(1).hand())
                .containsExactly(KING_CLUBS, SEVEN_DIAMONDS, NINE_DIAMONDS, SEVEN_CLUBS);
        assertThat(next.table()).isEmpty();
        assertThat(next.anyPileDiscarded()).isFalse();
    }

    @DisplayName("Should let attackers keep adding cards When the take has been announced")
    @Test
    void shouldLetAttackersKeepAddingCardsWhenTheTakeHasBeenAnnounced() {
        final DealState state = aDeal()
                .withEmptyDeck()
                .withPhase(DealPhase.DEFEND)
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(0, SEVEN_CLUBS)
                .withHand(1, KING_CLUBS)
                .withHand(2, SEVEN_SPADES)
                .build();

        final DealState announced = applied(engine.apply(state, new DealCommand.Take(1)));
        final DealState next = applied(engine.apply(announced, new DealCommand.Attack(0, SEVEN_CLUBS)));

        assertThat(next.phase()).isEqualTo(DealPhase.TAKING);
        assertThat(next.attackCardCount()).isEqualTo(2);
    }

    @DisplayName("Should ignore the taker's hand size When cards are added after the take")
    @Test
    void shouldIgnoreTheTakersHandSizeWhenCardsAreAddedAfterTheTake() {
        final DealState state = aDeal()
                .withEmptyDeck()
                .withPhase(DealPhase.DEFEND)
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(0, SEVEN_CLUBS)
                .withHand(1)
                .withHand(2, SEVEN_SPADES)
                .build();

        assertThat(state.defender().defendableCards(true)).isZero();

        final DealState announced = applied(engine.apply(state, new DealCommand.Take(1)));

        assertThat(engine.apply(announced, new DealCommand.Attack(0, SEVEN_CLUBS)).isApplied()).isTrue();
    }

    @DisplayName("Should still cap the follow-up cards at the round limit When the take has been announced")
    @Test
    void shouldStillCapTheFollowUpCardsAtTheRoundLimitWhenTheTakeHasBeenAnnounced() {
        final RulesConfig config = RulesConfig.defaults();
        final DealStateFixture fixture = aDeal().withEmptyDeck().withPhase(DealPhase.DEFEND);
        for (int index = 0; index < config.maxAttackFirstRound(); index++) {
            fixture.withAttackCards(PipCard.of(Rank.values()[index], Suit.DIAMONDS));
        }
        final DealState state = fixture
                .withHand(0, PipCard.of(Rank.SIX, Suit.HEARTS))
                .withHand(1, KING_CLUBS)
                .build();

        final DealState announced = applied(engine.apply(state, new DealCommand.Take(1)));

        assertThat(engine.apply(announced, new DealCommand.Attack(0, PipCard.of(Rank.SIX, Suit.HEARTS))))
                .isEqualTo(MoveResult.rejected(RejectionReason.ATTACK_LIMIT_REACHED));
    }

    @DisplayName("Should refuse a defence When the take has already been announced")
    @Test
    void shouldRefuseADefenceWhenTheTakeHasAlreadyBeenAnnounced() {
        final DealState state = aDeal()
                .withEmptyDeck()
                .withPhase(DealPhase.DEFEND)
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(0, SEVEN_CLUBS)
                .withHand(1, NINE_DIAMONDS)
                .build();

        final DealState announced = applied(engine.apply(state, new DealCommand.Take(1)));

        assertThat(engine.apply(announced, new DealCommand.Defend(1, NINE_DIAMONDS, SEVEN_DIAMONDS)))
                .isEqualTo(MoveResult.rejected(RejectionReason.DEFENDER_ALREADY_TOOK));
    }

    @DisplayName("Should refuse a transfer When the take has already been announced")
    @Test
    void shouldRefuseATransferWhenTheTakeHasAlreadyBeenAnnounced() {
        final DealState state = aDeal()
                .withEmptyDeck()
                .withPhase(DealPhase.DEFEND)
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(0, ACE_CLUBS)
                .withHand(1, SEVEN_CLUBS)
                .withHand(2, KING_CLUBS, SEVEN_SPADES)
                .build();

        final DealState announced = applied(engine.apply(state, new DealCommand.Take(1)));

        assertThat(engine.apply(announced, new DealCommand.Transfer(1, SEVEN_CLUBS)))
                .isEqualTo(MoveResult.rejected(RejectionReason.DEFENDER_ALREADY_TOOK));
    }

    @DisplayName("Should skip the defender in the next round When the defender took the table")
    @Test
    void shouldSkipTheDefenderInTheNextRoundWhenTheDefenderTookTheTable() {
        final DealState state = aDeal()
                .withEmptyDeck()
                .withPhase(DealPhase.DEFEND)
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(0, ACE_CLUBS)
                .withHand(1, KING_CLUBS)
                .withHand(2, SEVEN_SPADES)
                .build();

        final DealState announced = applied(engine.apply(state, new DealCommand.Take(1)));
        final DealState afterFirstPass = applied(engine.apply(announced, new DealCommand.Pass(0)));
        final DealState next = applied(engine.apply(afterFirstPass, new DealCommand.Pass(2)));

        assertThat(next.roundStarterSeat()).isEqualTo(2);
        assertThat(next.defenderSeat()).isZero();
    }

    @DisplayName("Should refuse any command When the deal is already over")
    @Test
    void shouldRefuseAnyCommandWhenTheDealIsAlreadyOver() {
        final DealState state = aDeal().withPhase(DealPhase.DEAL_OVER).withHand(0, ACE_CLUBS).build();

        assertThat(engine.apply(state, new DealCommand.Attack(0, ACE_CLUBS)))
                .isEqualTo(MoveResult.rejected(RejectionReason.NOT_YOUR_TURN));
    }

    @DisplayName("Should reveal the face-down card When it is played as the last card")
    @Test
    void shouldRevealTheFaceDownCardWhenItIsPlayedAsTheLastCard() {
        final DealState state = aDeal()
                .withEmptyDeck()
                .withFaceDownCard(0, SEVEN_DIAMONDS)
                .withHand(1, ACE_CLUBS)
                .build();

        final MoveResult result = engine.apply(state, new DealCommand.RevealFaceDown(0));

        final DealState next = applied(result);
        assertThat(next.playerAt(0).hasFaceDownCard()).isFalse();
        assertThat(next.table()).singleElement()
                .satisfies(slot -> assertThat(slot.attack()).isEqualTo(SEVEN_DIAMONDS));
        assertThat(events(result)).containsExactly(
                new DealEvent.FaceDownRevealed(0, SEVEN_DIAMONDS),
                new DealEvent.CardAttacked(0, SEVEN_DIAMONDS));
    }

    private static DealState applied(final MoveResult result) {
        assertThat(result).isInstanceOf(MoveResult.Applied.class);
        return ((MoveResult.Applied) result).state();
    }

    private static List<DealEvent> events(final MoveResult result) {
        return ((MoveResult.Applied) result).events();
    }
}
