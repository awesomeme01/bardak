package kz.bardak.game.rules;

import static kz.bardak.game.rules.DealStateFixture.aDeal;
import static org.assertj.core.api.Assertions.assertThat;

import java.util.List;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

/**
 * Добор, выход игроков и конец раздачи — §1.4.1, §1.7, §1.8, ADR-033.
 */
class DealRefillAndExitTest {

    private static final Card SEVEN_DIAMONDS = PipCard.of(Rank.SEVEN, Suit.DIAMONDS);
    private static final Card NINE_DIAMONDS = PipCard.of(Rank.NINE, Suit.DIAMONDS);
    private static final Card ACE_CLUBS = PipCard.of(Rank.ACE, Suit.CLUBS);
    private static final Card KING_CLUBS = PipCard.of(Rank.KING, Suit.CLUBS);
    private static final Card QUEEN_CLUBS = PipCard.of(Rank.QUEEN, Suit.CLUBS);

    private final DealEngine engine = DealEngine.withDefaults();

    @DisplayName("Should give the scarce cards to the round starter first When the deck runs short")
    @Test
    void shouldGiveTheScarceCardsToTheRoundStarterFirstWhenTheDeckRunsShort() {
        final DealState state = aDeal()
                .withDeckOf(2)
                .withBeatenPair(SEVEN_DIAMONDS, NINE_DIAMONDS)
                .withHand(0, ACE_CLUBS)
                .withHand(1, KING_CLUBS)
                .withHand(2, QUEEN_CLUBS)
                .build();

        final DealState afterFirstPass = applied(engine.apply(state, new DealCommand.Pass(0)));
        final DealState next = applied(engine.apply(afterFirstPass, new DealCommand.Pass(2)));

        assertThat(next.playerAt(0).handSize()).isEqualTo(3);
        assertThat(next.playerAt(2).handSize()).isEqualTo(1);
        assertThat(next.playerAt(1).handSize()).isEqualTo(1);
        assertThat(next.isDeckEmpty()).isTrue();
    }

    @DisplayName("Should refill the defender last When the deck has just enough for the others")
    @Test
    void shouldRefillTheDefenderLastWhenTheDeckHasJustEnoughForTheOthers() {
        final DealState state = aDeal()
                .withDeckOf(1)
                .withBeatenPair(SEVEN_DIAMONDS, NINE_DIAMONDS)
                .withHand(0, ACE_CLUBS)
                .withHand(1, KING_CLUBS)
                .withHand(2, QUEEN_CLUBS)
                .build();

        final DealState afterFirstPass = applied(engine.apply(state, new DealCommand.Pass(0)));
        final DealState next = applied(engine.apply(afterFirstPass, new DealCommand.Pass(2)));

        assertThat(next.playerAt(0).handSize()).isEqualTo(2);
        assertThat(next.playerAt(1).handSize()).isEqualTo(1);
    }

    @DisplayName("Should let the attacker leave first and lose the defender When both hands empty at once")
    @Test
    void shouldLetTheAttackerLeaveFirstAndLoseTheDefenderWhenBothHandsEmptyAtOnce() {
        final DealState state = aDeal()
                .withSeats(2)
                .withEmptyDeck()
                .withHand(0, SEVEN_DIAMONDS)
                .withHand(1, NINE_DIAMONDS)
                .build();

        final DealState attacked = applied(engine.apply(state, new DealCommand.Attack(0, SEVEN_DIAMONDS)));
        final DealState defended = applied(
                engine.apply(attacked, new DealCommand.Defend(1, NINE_DIAMONDS, SEVEN_DIAMONDS)));
        final MoveResult result = engine.apply(defended, new DealCommand.Pass(0));

        final DealState next = applied(result);
        assertThat(next.phase()).isEqualTo(DealPhase.DEAL_OVER);
        assertThat(next.exitOrder()).containsExactly(0);
        assertThat(next.playerAt(1).inDeal()).isTrue();
        assertThat(next.playerAt(1).handSize()).isZero();
        assertThat(events(result)).contains(new DealEvent.PlayerLeftDeal(0), new DealEvent.DealFinished(1));
    }

    @DisplayName("Should keep a player in the deal When the face-down card is still unopened")
    @Test
    void shouldKeepAPlayerInTheDealWhenTheFaceDownCardIsStillUnopened() {
        final DealState state = aDeal()
                .withSeats(2)
                .withEmptyDeck()
                .withHand(0, SEVEN_DIAMONDS)
                .withFaceDownCard(0, ACE_CLUBS)
                .withHand(1, NINE_DIAMONDS, KING_CLUBS)
                .build();

        final DealState attacked = applied(engine.apply(state, new DealCommand.Attack(0, SEVEN_DIAMONDS)));
        final DealState defended = applied(
                engine.apply(attacked, new DealCommand.Defend(1, NINE_DIAMONDS, SEVEN_DIAMONDS)));
        final DealState next = applied(engine.apply(defended, new DealCommand.Pass(0)));

        assertThat(next.phase()).isEqualTo(DealPhase.ATTACK);
        assertThat(next.playerAt(0).inDeal()).isTrue();
        assertThat(next.exitOrder()).isEmpty();
    }

    @DisplayName("Should not let a player leave While the deck still has cards to draw")
    @Test
    void shouldNotLetAPlayerLeaveWhileTheDeckStillHasCardsToDraw() {
        final DealState state = aDeal()
                .withSeats(2)
                .withDeckOf(3)
                .withHand(0, SEVEN_DIAMONDS)
                .withHand(1, NINE_DIAMONDS, KING_CLUBS)
                .build();

        final DealState attacked = applied(engine.apply(state, new DealCommand.Attack(0, SEVEN_DIAMONDS)));
        final DealState defended = applied(
                engine.apply(attacked, new DealCommand.Defend(1, NINE_DIAMONDS, SEVEN_DIAMONDS)));
        final DealState next = applied(engine.apply(defended, new DealCommand.Pass(0)));

        assertThat(next.exitOrder()).isEmpty();
        assertThat(next.playerAt(0).handSize()).isEqualTo(3);
        assertThat(next.isDeckEmpty()).isTrue();
    }

    @DisplayName("Should record every exit in order When players leave across rounds")
    @Test
    void shouldRecordEveryExitInOrderWhenPlayersLeaveAcrossRounds() {
        final DealState state = aDeal()
                .withEmptyDeck()
                .withHand(0, SEVEN_DIAMONDS)
                .withHand(1, NINE_DIAMONDS)
                .withHand(2, ACE_CLUBS, KING_CLUBS)
                .build();

        final DealState attacked = applied(engine.apply(state, new DealCommand.Attack(0, SEVEN_DIAMONDS)));
        final DealState defended = applied(
                engine.apply(attacked, new DealCommand.Defend(1, NINE_DIAMONDS, SEVEN_DIAMONDS)));
        final DealState afterFirstPass = applied(engine.apply(defended, new DealCommand.Pass(0)));
        final DealState next = applied(engine.apply(afterFirstPass, new DealCommand.Pass(2)));

        assertThat(next.exitOrder()).containsExactly(0, 1);
        assertThat(next.phase()).isEqualTo(DealPhase.DEAL_OVER);
        assertThat(next.playerAt(2).inDeal()).isTrue();
    }

    @DisplayName("Should keep the taken cards out of the exit check When the defender takes the table")
    @Test
    void shouldKeepTheTakenCardsOutOfTheExitCheckWhenTheDefenderTakesTheTable() {
        final DealState state = aDeal()
                .withSeats(2)
                .withEmptyDeck()
                .withPhase(DealPhase.DEFEND)
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(0, ACE_CLUBS)
                .build();

        final DealState announced = applied(engine.apply(state, new DealCommand.Take(1)));
        final MoveResult result = engine.apply(announced, new DealCommand.Pass(0));

        final DealState next = applied(result);
        assertThat(next.playerAt(1).hand()).containsExactly(SEVEN_DIAMONDS);
        assertThat(next.playerAt(1).inDeal()).isTrue();
        assertThat(next.exitOrder()).isEmpty();
        assertThat(next.phase()).isEqualTo(DealPhase.ATTACK);
    }

    private static DealState applied(final MoveResult result) {
        assertThat(result).isInstanceOf(MoveResult.Applied.class);
        return ((MoveResult.Applied) result).state();
    }

    private static List<DealEvent> events(final MoveResult result) {
        return ((MoveResult.Applied) result).events();
    }
}
