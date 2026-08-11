package kz.bardak.game.rules;

import static kz.bardak.game.rules.DealStateFixture.aDeal;
import static org.assertj.core.api.Assertions.assertThat;

import java.util.List;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

/**
 * Потайной козырь — §1.9, ADR-035. Он лежит ниже козырной карты, достаётся последнему
 * добирающему и меняет козырь <b>со следующего раунда</b>.
 */
class HiddenTrumpTest {

    private static final Card SEVEN_DIAMONDS = PipCard.of(Rank.SEVEN, Suit.DIAMONDS);
    private static final Card NINE_DIAMONDS = PipCard.of(Rank.NINE, Suit.DIAMONDS);
    /**
     * Карта над потайным козырём. Масть намеренно отличается и от текущего козыря, и от
     * потайного: если движок вскроет её по ошибке, это сразу видно по сменившейся масти.
     */
    private static final Card TRUMP_CARD = PipCard.of(Rank.KING, Suit.DIAMONDS);
    private static final Card HIDDEN_TRUMP = PipCard.of(Rank.SIX, Suit.SPADES);

    private final DealEngine engine = DealEngine.withDefaults();

    @DisplayName("Should change the trump from the next round When the hidden trump is drawn")
    @Test
    void shouldChangeTheTrumpFromTheNextRoundWhenTheHiddenTrumpIsDrawn() {
        final MoveResult result = closeRound(dealWithHiddenTrump(HIDDEN_TRUMP));

        final DealState next = applied(result);
        assertThat(next.trump().suit()).isEqualTo(Suit.SPADES);
        assertThat(next.trump().protectedSuit()).isEqualTo(Suit.CLUBS);
        assertThat(next.table()).isEmpty();
        assertThat(next.phase()).isEqualTo(DealPhase.ATTACK);
    }

    @DisplayName("Should give the trump card to the previous drawer When the deck runs out")
    @Test
    void shouldGiveTheTrumpCardToThePreviousDrawerWhenTheDeckRunsOut() {
        final DealState next = applied(closeRound(dealWithHiddenTrump(HIDDEN_TRUMP)));

        assertThat(next.playerAt(0).hand()).contains(TRUMP_CARD);
        assertThat(next.playerAt(1).hand()).contains(HIDDEN_TRUMP);
        assertThat(next.isDeckEmpty()).isTrue();
    }

    @DisplayName("Should show the hidden trump to everybody When it is revealed")
    @Test
    void shouldShowTheHiddenTrumpToEverybodyWhenItIsRevealed() {
        final MoveResult result = closeRound(dealWithHiddenTrump(HIDDEN_TRUMP));

        assertThat(events(result))
                .contains(new DealEvent.HiddenTrumpRevealed(1, HIDDEN_TRUMP))
                .contains(new DealEvent.TrumpChanged(1, Suit.SPADES));
        assertThat(events(result)).filteredOn(DealEvent.HiddenTrumpRevealed.class::isInstance)
                .withFailMessage("Вскрыться должен ровно один потайной козырь — самая нижняя карта")
                .hasSize(1);
        assertThat(new DealEvent.HiddenTrumpRevealed(1, HIDDEN_TRUMP).privateToSeat()).isEmpty();
    }

    @DisplayName("Should keep the hidden trump secret When the deal ended before it was reached")
    @Test
    void shouldKeepTheHiddenTrumpSecretWhenTheDealEndedBeforeItWasReached() {
        final DealState state = aDeal()
                .withDeck(PipCard.of(Rank.TEN, Suit.CLUBS), TRUMP_CARD, HIDDEN_TRUMP)
                .withBeatenPair(SEVEN_DIAMONDS, NINE_DIAMONDS)
                .withHand(0, fiveCards())
                .withHand(1, sixCards())
                .withHand(2, sixCards())
                .build();

        final DealState next = applied(closeRound(state));

        assertThat(next.deck()).containsExactly(TRUMP_CARD, HIDDEN_TRUMP);
        assertThat(next.trump().suit()).isEqualTo(Suit.HEARTS);
    }

    @DisplayName("Should hold the joker aside until the suit is named When the hidden trump is a joker")
    @Test
    void shouldHoldTheJokerAsideUntilTheSuitIsNamedWhenTheHiddenTrumpIsAJoker() {
        final Card joker = new JokerCard(1);

        final DealState next = applied(closeRound(dealWithHiddenTrump(joker)));

        assertThat(next.phase()).isEqualTo(DealPhase.DICE);
        assertThat(next.hiddenTrumpAwaitingSuit()).get()
                .satisfies(pending -> assertThat(pending.card()).isEqualTo(joker));
        assertThat(next.players()).allSatisfy(player -> assertThat(player.hand()).doesNotContain(joker));
        assertThat(next.trump().suit()).isEqualTo(Suit.HEARTS);
    }

    @DisplayName("Should hand the joker over after the suit is named When the dice winner chooses")
    @Test
    void shouldHandTheJokerOverAfterTheSuitIsNamedWhenTheDiceWinnerChooses() {
        final Card joker = new JokerCard(1);
        final DealState waiting = applied(closeRound(dealWithHiddenTrump(joker)));
        final int chooser = waiting.hiddenTrumpAwaitingSuit().orElseThrow().chooserSeat();

        final DealState next = applied(engine.apply(waiting, new DealCommand.ChooseTrump(chooser, Suit.CLUBS)));

        assertThat(next.trump().suit()).isEqualTo(Suit.CLUBS);
        assertThat(next.trump().protectedSuit()).isEqualTo(Suit.SPADES);
        assertThat(next.playerAt(1).hand()).contains(joker);
        assertThat(next.hiddenTrumpAwaitingSuit()).isEmpty();
        assertThat(next.phase()).isEqualTo(DealPhase.ATTACK);
    }

    @DisplayName("Should refuse the choice from anybody else When the dice picked a chooser")
    @Test
    void shouldRefuseTheChoiceFromAnybodyElseWhenTheDicePickedAChooser() {
        final DealState waiting = applied(closeRound(dealWithHiddenTrump(new JokerCard(1))));
        final int chooser = waiting.hiddenTrumpAwaitingSuit().orElseThrow().chooserSeat();
        final int other = (chooser + 1) % waiting.players().size();

        assertThat(engine.apply(waiting, new DealCommand.ChooseTrump(other, Suit.CLUBS)))
                .isEqualTo(MoveResult.rejected(RejectionReason.NOT_YOUR_TURN));
    }

    @DisplayName("Should refuse to play at all While the trump is not named")
    @Test
    void shouldRefuseToPlayAtAllWhileTheTrumpIsNotNamed() {
        final DealState waiting = applied(closeRound(dealWithHiddenTrump(new JokerCard(1))));
        final Card card = waiting.playerAt(waiting.attackRightSeat()).hand().get(0);

        assertThat(engine.apply(waiting, new DealCommand.Attack(waiting.attackRightSeat(), card)))
                .isEqualTo(MoveResult.rejected(RejectionReason.TRUMP_NOT_CHOSEN_YET));
    }

    /**
     * Раунд отбит, в колоде остались ровно две карты: козырная и под ней потайной козырь.
     * Один пас закрывает раунд, и обе уходят в добор.
     */
    private static DealState dealWithHiddenTrump(final Card hiddenTrump) {
        return aDeal()
                .withDeck(TRUMP_CARD, hiddenTrump)
                .withBeatenPair(SEVEN_DIAMONDS, NINE_DIAMONDS)
                .withHand(0, fiveCards())
                .withHand(1, fiveCards())
                .withHand(2, sixCards())
                .build();
    }

    /**
     * Раунд закрывается только когда спасовали все, кто имеет право подкидывать: начавший
     * раунд и второй сосед (§2.1).
     */
    private MoveResult closeRound(final DealState state) {
        final DealState afterFirstPass = applied(engine.apply(state, new DealCommand.Pass(0)));
        return engine.apply(afterFirstPass, new DealCommand.Pass(2));
    }

    private static Card[] fiveCards() {
        return new Card[]{
                PipCard.of(Rank.SIX, Suit.CLUBS), PipCard.of(Rank.SEVEN, Suit.CLUBS),
                PipCard.of(Rank.EIGHT, Suit.CLUBS), PipCard.of(Rank.NINE, Suit.CLUBS),
                PipCard.of(Rank.TEN, Suit.CLUBS)};
    }

    private static Card[] sixCards() {
        return new Card[]{
                PipCard.of(Rank.SIX, Suit.DIAMONDS), PipCard.of(Rank.EIGHT, Suit.DIAMONDS),
                PipCard.of(Rank.TEN, Suit.DIAMONDS), PipCard.of(Rank.JACK, Suit.DIAMONDS),
                PipCard.of(Rank.QUEEN, Suit.DIAMONDS), PipCard.of(Rank.KING, Suit.DIAMONDS)};
    }

    private static DealState applied(final MoveResult result) {
        assertThat(result).isInstanceOf(MoveResult.Applied.class);
        return ((MoveResult.Applied) result).state();
    }

    private static List<DealEvent> events(final MoveResult result) {
        return ((MoveResult.Applied) result).events();
    }
}
