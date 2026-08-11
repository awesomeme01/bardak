package kz.bardak.game.runtime;

import static org.assertj.core.api.Assertions.assertThat;

import kz.bardak.game.rules.Card;
import kz.bardak.game.rules.DealCommand;
import kz.bardak.game.rules.DealPhase;
import kz.bardak.game.rules.DealState;
import kz.bardak.game.rules.DealStateFixtureAccess;
import kz.bardak.game.rules.PipCard;
import kz.bardak.game.rules.Rank;
import kz.bardak.game.rules.Suit;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

/**
 * Автодействие по таймауту (§5.1). Движок никогда не выбирает за человека карту —
 * он выбирает самое безобидное действие.
 */
class TimeoutPolicyTest {

    private static final Card SEVEN_DIAMONDS = PipCard.of(Rank.SEVEN, Suit.DIAMONDS);
    private static final Card NINE_DIAMONDS = PipCard.of(Rank.NINE, Suit.DIAMONDS);

    @DisplayName("Should pass for the attacker When the attack times out")
    @Test
    void shouldPassForTheAttackerWhenTheAttackTimesOut() {
        final DealState deal = DealStateFixtureAccess.attacking();

        assertThat(TimeoutPolicy.seatOnTheClock(deal)).contains(deal.attackRightSeat());
        assertThat(TimeoutPolicy.autoActionFor(deal))
                .contains(new DealCommand.Pass(deal.attackRightSeat()));
    }

    @DisplayName("Should take for the defender When the defence times out")
    @Test
    void shouldTakeForTheDefenderWhenTheDefenceTimesOut() {
        final DealState deal = DealStateFixtureAccess.defending(SEVEN_DIAMONDS, NINE_DIAMONDS);

        assertThat(TimeoutPolicy.autoActionFor(deal))
                .contains(new DealCommand.Take(deal.defenderSeat()));
    }

    @DisplayName("Should skip the hanging When nobody decides in time")
    @Test
    void shouldSkipTheHangingWhenNobodyDecidesInTime() {
        final DealState deal = DealStateFixtureAccess.hanging();

        assertThat(TimeoutPolicy.autoActionFor(deal)).get()
                .isInstanceOf(DealCommand.HangSkip.class);
    }

    @DisplayName("Should choose the richest suit When the trump chooser stays silent")
    @Test
    void shouldChooseTheRichestSuitWhenTheTrumpChooserStaysSilent() {
        final DealState deal = DealStateFixtureAccess.choosingTrump();

        assertThat(TimeoutPolicy.autoActionFor(deal)).get()
                .isInstanceOfSatisfying(DealCommand.ChooseTrump.class,
                        command -> assertThat(command.suit()).isEqualTo(Suit.CLUBS));
    }

    @DisplayName("Should wait for nobody When the deal is over")
    @Test
    void shouldWaitForNobodyWhenTheDealIsOver() {
        final DealState deal = DealStateFixtureAccess.finished();

        assertThat(TimeoutPolicy.seatOnTheClock(deal)).isEmpty();
        assertThat(TimeoutPolicy.autoActionFor(deal)).isEmpty();
    }

    @DisplayName("Should pass instead of taking When the table is empty")
    @Test
    void shouldPassInsteadOfTakingWhenTheTableIsEmpty() {
        final DealState deal = DealStateFixtureAccess.defendingEmptyTable();

        assertThat(TimeoutPolicy.autoActionFor(deal))
                .contains(new DealCommand.Pass(deal.defenderSeat()));
    }
}
