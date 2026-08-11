package kz.bardak.game.rules;

import static kz.bardak.game.rules.DealStateFixture.aDeal;
import static org.assertj.core.api.Assertions.assertThat;

import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

/**
 * Итог раздачи — §0.1, §0.3, §0.4. Считать поигроково нельзя: судьба навесившего зависит
 * от судьбы того, кому он навесил.
 */
class DealScoringTest {

    private static final int ACE_LEVEL = NavesScale.full().jokerLevel() - 1;
    private static final int JOKER_LEVEL = NavesScale.full().jokerLevel();

    private static final Card EIGHT_DIAMONDS = PipCard.of(Rank.EIGHT, Suit.DIAMONDS);
    private static final Card EIGHT_HEARTS = PipCard.of(Rank.EIGHT, Suit.HEARTS);
    private static final Card EIGHT_SPADES = PipCard.of(Rank.EIGHT, Suit.SPADES);
    private static final Card EIGHT_CLUBS = PipCard.of(Rank.EIGHT, Suit.CLUBS);
    private static final Card NINE_CLUBS = PipCard.of(Rank.NINE, Suit.CLUBS);

    private final DealScoring scoring = new DealScoring(RulesConfig.defaults());

    @DisplayName("Should add one step to the deal loser When the deal ends")
    @Test
    void shouldAddOneStepToTheDealLoserWhenTheDealEnds() {
        final DealOutcome outcome = scoring.score(finishedDeal()
                .withNavesLevel(1, 2)
                .withExitOrder(0, 2)
                .build());

        assertThat(outcome.dealLoserSeat()).isEqualTo(1);
        assertThat(outcome.forSeat(1).levelAfter()).isEqualTo(3);
    }

    @DisplayName("Should take one step off the first player out When the deal ends")
    @Test
    void shouldTakeOneStepOffTheFirstPlayerOutWhenTheDealEnds() {
        final DealOutcome outcome = scoring.score(finishedDeal()
                .withNavesLevel(0, 4)
                .withNavesLevel(2, 4)
                .withExitOrder(0, 2)
                .build());

        assertThat(outcome.forSeat(0).levelAfter()).isEqualTo(3);
        assertThat(outcome.forSeat(2).levelAfter()).isEqualTo(4);
    }

    @DisplayName("Should keep the floor When the first player out has nothing hung")
    @Test
    void shouldKeepTheFloorWhenTheFirstPlayerOutHasNothingHung() {
        final DealOutcome outcome = scoring.score(finishedDeal()
                .withExitOrder(0, 2)
                .build());

        assertThat(outcome.forSeat(0).levelAfter()).isEqualTo(NavesScale.NO_NAVES);
        assertThat(outcome.forSeat(0).shift()).isZero();
    }

    @DisplayName("Should turn the ace into a joker When the deal loser stands on the ace")
    @Test
    void shouldTurnTheAceIntoAJokerWhenTheDealLoserStandsOnTheAce() {
        final DealOutcome outcome = scoring.score(finishedDeal()
                .withNavesLevel(1, ACE_LEVEL)
                .withExitOrder(0, 2)
                .build());

        assertThat(outcome.forSeat(1).levelAfter()).isEqualTo(JOKER_LEVEL);
        assertThat(outcome.forSeat(1).degree()).contains(LossDegree.SUPER_FAIL);
        assertThat(outcome.isMatchOver()).isTrue();
    }

    @DisplayName("Should strip the joker When the player with it went out first")
    @Test
    void shouldStripTheJokerWhenThePlayerWithItWentOutFirst() {
        final DealOutcome outcome = scoring.score(finishedDeal()
                .withJokerHungBy(0, 2)
                .withExitOrder(0, 2)
                .build());

        assertThat(outcome.forSeat(0).levelAfter()).isEqualTo(ACE_LEVEL);
        assertThat(outcome.forSeat(0).isLoser()).isFalse();
        assertThat(outcome.isMatchOver()).isFalse();
    }

    @DisplayName("Should lose the game When the joker holder did not go out first")
    @Test
    void shouldLoseTheGameWhenTheJokerHolderDidNotGoOutFirst() {
        final DealOutcome outcome = scoring.score(finishedDeal()
                .withJokerHungBy(2, 0)
                .withExitOrder(0, 2)
                .build());

        assertThat(outcome.forSeat(2).degree()).contains(LossDegree.FAIL);
        assertThat(outcome.isMatchOver()).isTrue();
    }

    @DisplayName("Should reward the finisher When the victim lost the game")
    @Test
    void shouldRewardTheFinisherWhenTheVictimLostTheGame() {
        final DealOutcome outcome = scoring.score(finishedDeal()
                .withNavesLevel(0, 4)
                .withJokerHungBy(2, 0)
                .withExitOrder(1)
                .build());

        assertThat(outcome.forSeat(0).levelAfter()).isEqualTo(3);
        assertThat(outcome.forSeat(2).isLoser()).isTrue();
    }

    @DisplayName("Should reward the finisher even with a joker of their own When the victim lost")
    @Test
    void shouldRewardTheFinisherEvenWithAJokerOfTheirOwnWhenTheVictimLost() {
        final DealOutcome outcome = scoring.score(aDeal()
                .withSeats(4)
                .withPhase(DealPhase.DEAL_OVER)
                .withEmptyDeck()
                .withPlayerOutOfDeal(0)
                .withPlayerOutOfDeal(2)
                .withPlayerOutOfDeal(3)
                .withJokerHungBy(0, 3)
                .withJokerHungBy(2, 0)
                .withExitOrder(3)
                .build());

        assertThat(outcome.forSeat(0).levelAfter()).isEqualTo(ACE_LEVEL);
        assertThat(outcome.forSeat(0).isLoser()).isFalse();
        assertThat(outcome.forSeat(2).isLoser()).isTrue();
    }

    @DisplayName("Should sum the rewards When the finisher killed two victims")
    @Test
    void shouldSumTheRewardsWhenTheFinisherKilledTwoVictims() {
        final DealOutcome outcome = scoring.score(aDeal()
                .withSeats(5)
                .withPhase(DealPhase.DEAL_OVER)
                .withEmptyDeck()
                .withPlayerOutOfDeal(0)
                .withPlayerOutOfDeal(2)
                .withPlayerOutOfDeal(3)
                .withPlayerOutOfDeal(4)
                .withNavesLevel(0, 5)
                .withJokerHungBy(2, 0)
                .withJokerHungBy(3, 0)
                .withExitOrder(4, 2, 3)
                .build());

        assertThat(outcome.losers()).extracting(PlayerOutcome::seatNo).containsExactlyInAnyOrder(2, 3);
        assertThat(outcome.forSeat(0).levelAfter()).isEqualTo(3);
        assertThat(outcome.forSeat(0).shift()).isEqualTo(-2);
    }

    @DisplayName("Should sum every shift of the deal When a player went out first and finished two")
    @Test
    void shouldSumEveryShiftOfTheDealWhenAPlayerWentOutFirstAndFinishedTwo() {
        final DealOutcome outcome = scoring.score(aDeal()
                .withSeats(4)
                .withPhase(DealPhase.DEAL_OVER)
                .withEmptyDeck()
                .withPlayerOutOfDeal(0)
                .withPlayerOutOfDeal(2)
                .withPlayerOutOfDeal(3)
                .withNavesLevel(0, 5)
                .withJokerHungBy(2, 0)
                .withJokerHungBy(3, 0)
                .withExitOrder(0, 2, 3)
                .build());

        assertThat(outcome.forSeat(0).shift()).isEqualTo(-3);
        assertThat(outcome.forSeat(0).levelAfter()).isEqualTo(2);
    }

    @DisplayName("Should apply the floor only at the end When shifts point both ways")
    @Test
    void shouldApplyTheFloorOnlyAtTheEndWhenShiftsPointBothWays() {
        final DealOutcome outcome = scoring.score(aDeal()
                .withSeats(4)
                .withPhase(DealPhase.DEAL_OVER)
                .withEmptyDeck()
                .withPlayerOutOfDeal(0)
                .withPlayerOutOfDeal(2)
                .withPlayerOutOfDeal(3)
                .withNavesLevel(1, 1)
                .withJokerHungBy(2, 1)
                .withJokerHungBy(3, 1)
                .withExitOrder(0)
                .build());

        assertThat(outcome.dealLoserSeat()).isEqualTo(1);
        assertThat(outcome.forSeat(1).levelAfter()).isZero();
    }

    @DisplayName("Should call it royal When the last attack was exactly four eights")
    @Test
    void shouldCallItRoyalWhenTheLastAttackWasExactlyFourEights() {
        final DealOutcome outcome = scoring.score(finishedDeal()
                .withNavesLevel(1, ACE_LEVEL)
                .withExitOrder(0, 2)
                .withLastAttack(EIGHT_DIAMONDS, EIGHT_HEARTS, EIGHT_SPADES, EIGHT_CLUBS)
                .build());

        assertThat(outcome.forSeat(1).degree()).contains(LossDegree.ROYAL);
    }

    @DisplayName("Should still call it royal When the loser beat all four eights")
    @Test
    void shouldStillCallItRoyalWhenTheLoserBeatAllFourEights() {
        final DealState state = finishedDeal()
                .withNavesLevel(1, ACE_LEVEL)
                .withExitOrder(0, 2)
                .withLastAttack(EIGHT_DIAMONDS, EIGHT_HEARTS, EIGHT_SPADES, EIGHT_CLUBS)
                .withHand(1, NINE_CLUBS)
                .build();

        assertThat(scoring.score(state).forSeat(1).degree()).contains(LossDegree.ROYAL);
    }

    @DisplayName("Should call it super mega suck When the last attack held one to three eights")
    @Test
    void shouldCallItSuperMegaSuckWhenTheLastAttackHeldOneToThreeEights() {
        final DealOutcome outcome = scoring.score(finishedDeal()
                .withNavesLevel(1, ACE_LEVEL)
                .withExitOrder(0, 2)
                .withLastAttack(EIGHT_DIAMONDS, NINE_CLUBS)
                .build());

        assertThat(outcome.forSeat(1).degree()).contains(LossDegree.SUPER_MEGA_SUCK);
    }

    @DisplayName("Should call it super mega fail When the joker arrived as a card without eights")
    @Test
    void shouldCallItSuperMegaFailWhenTheJokerArrivedAsACardWithoutEights() {
        final DealOutcome outcome = scoring.score(finishedDeal()
                .withJokerHungBy(1, 0)
                .withExitOrder(0, 2)
                .withLastAttack(NINE_CLUBS)
                .build());

        assertThat(outcome.forSeat(1).degree()).contains(LossDegree.SUPER_MEGA_FAIL);
    }

    @DisplayName("Should prefer the heavier degree When the joker came both by card and by plus one")
    @Test
    void shouldPreferTheHeavierDegreeWhenTheJokerCameBothByCardAndByPlusOne() {
        final DealOutcome outcome = scoring.score(finishedDeal()
                .withJokerHungBy(1, 0)
                .withExitOrder(0, 2)
                .build());

        assertThat(outcome.forSeat(1).degree()).contains(LossDegree.SUPER_MEGA_FAIL);
    }

    @DisplayName("Should prefer royal over super fail When the joker came by plus one but eights were played")
    @Test
    void shouldPreferRoyalOverSuperFailWhenTheJokerCameByPlusOneButEightsWerePlayed() {
        final DealOutcome outcome = scoring.score(finishedDeal()
                .withNavesLevel(1, ACE_LEVEL)
                .withExitOrder(0, 2)
                .withLastAttack(EIGHT_DIAMONDS, EIGHT_HEARTS, EIGHT_SPADES, EIGHT_CLUBS)
                .build());

        assertThat(outcome.forSeat(1).degree()).contains(LossDegree.ROYAL);
    }

    @DisplayName("Should prefer royal over super mega fail When the joker came by card and eights were played")
    @Test
    void shouldPreferRoyalOverSuperMegaFailWhenTheJokerCameByCardAndEightsWerePlayed() {
        final DealOutcome outcome = scoring.score(finishedDeal()
                .withJokerHungBy(1, 0)
                .withExitOrder(0, 2)
                .withLastAttack(EIGHT_DIAMONDS, EIGHT_HEARTS, EIGHT_SPADES, EIGHT_CLUBS)
                .build());

        assertThat(outcome.forSeat(1).degree()).contains(LossDegree.ROYAL);
    }

    @DisplayName("Should prefer super mega suck over super mega fail When the joker came by card with one eight")
    @Test
    void shouldPreferSuperMegaSuckOverSuperMegaFailWhenTheJokerCameByCardWithOneEight() {
        final DealOutcome outcome = scoring.score(finishedDeal()
                .withJokerHungBy(1, 0)
                .withExitOrder(0, 2)
                .withLastAttack(EIGHT_DIAMONDS, NINE_CLUBS)
                .build());

        assertThat(outcome.forSeat(1).degree()).contains(LossDegree.SUPER_MEGA_SUCK);
    }

    @DisplayName("Should pick the main loser by degree When several players lost the game")
    @Test
    void shouldPickTheMainLoserByDegreeWhenSeveralPlayersLostTheGame() {
        final DealOutcome outcome = scoring.score(aDeal()
                .withSeats(4)
                .withPhase(DealPhase.DEAL_OVER)
                .withEmptyDeck()
                .withPlayerOutOfDeal(0)
                .withPlayerOutOfDeal(2)
                .withPlayerOutOfDeal(3)
                .withNavesLevel(1, ACE_LEVEL)
                .withJokerHungBy(2, 0)
                .withJokerHungBy(3, 0)
                .withExitOrder(0)
                .withLastAttack(EIGHT_DIAMONDS, EIGHT_HEARTS, EIGHT_SPADES, EIGHT_CLUBS)
                .build());

        assertThat(outcome.losers()).hasSize(3);
        assertThat(outcome.mainLoser()).get().satisfies(loser -> {
            assertThat(loser.seatNo()).isEqualTo(1);
            assertThat(loser.lossDegree()).isEqualTo(LossDegree.ROYAL);
        });
    }

    @DisplayName("Should leave everyone else untouched When only the loser and the first out shift")
    @Test
    void shouldLeaveEveryoneElseUntouchedWhenOnlyTheLoserAndTheFirstOutShift() {
        final DealOutcome outcome = scoring.score(finishedDeal()
                .withNavesLevel(2, 4)
                .withExitOrder(0, 2)
                .build());

        assertThat(outcome.forSeat(2).shift()).isZero();
    }

    @DisplayName("Should explain both shifts separately When a player loses and finishes somebody")
    @Test
    void shouldExplainBothShiftsSeparatelyWhenAPlayerLosesAndFinishesSomebody() {
        // Место 1 проиграло раздачу (+1) и добило место 2 джокером (−1): сумма нулевая,
        // и по одному только уровню обе причины были бы неразличимы.
        final DealOutcome outcome = scoring.score(aDeal()
                .withPhase(DealPhase.DEAL_OVER)
                .withEmptyDeck()
                .withPlayerOutOfDeal(0)
                .withNavesLevel(1, 3)
                .withJokerHungBy(2, 1)
                .withExitOrder(0)
                .build());

        assertThat(outcome.forSeat(1).shift()).isZero();
        assertThat(outcome.forSeat(1).changes()).containsExactly(
                new LevelChange(LevelChangeReason.LOST_DEAL, 1),
                new LevelChange(LevelChangeReason.FINISHED_OPPONENT, -1));
    }

    @DisplayName("Should record the scale limit When the floor swallows a step")
    @Test
    void shouldRecordTheScaleLimitWhenTheFloorSwallowsAStep() {
        final DealOutcome outcome = scoring.score(finishedDeal()
                .withExitOrder(0, 2)
                .build());

        // «Летит 6» — нижняя ступень: −1 отсюда некуда, и это должно быть видно в истории.
        assertThat(outcome.forSeat(0).changes()).containsExactly(
                new LevelChange(LevelChangeReason.FIRST_OUT, -1),
                new LevelChange(LevelChangeReason.SCALE_LIMIT, 1));
    }

    @DisplayName("Should place the players by their exit order When the deal ends")
    @Test
    void shouldPlaceThePlayersByTheirExitOrderWhenTheDealEnds() {
        final DealOutcome outcome = scoring.score(finishedDeal()
                .withExitOrder(2, 0)
                .build());

        assertThat(outcome.forSeat(2).place()).isEqualTo(1);
        assertThat(outcome.forSeat(0).place()).isEqualTo(2);
        // Оставшийся с картами — последний, в порядке выхода его нет вовсе.
        assertThat(outcome.forSeat(1).place()).isEqualTo(3);
    }

    @DisplayName("Should carry the last attack and the trump When the deal is scored")
    @Test
    void shouldCarryTheLastAttackAndTheTrumpWhenTheDealIsScored() {
        final DealOutcome outcome = scoring.score(finishedDeal()
                .withExitOrder(0, 2)
                .withLastAttack(EIGHT_DIAMONDS, NINE_CLUBS)
                .build());

        // ⭐ После подсчёта раздача исчезает, поэтому обстановку обязан нести итог.
        assertThat(outcome.lastAttackCards()).containsExactly(EIGHT_DIAMONDS, NINE_CLUBS);
        assertThat(outcome.trumpSuit()).isNotNull();
    }

    /** Раздача закончилась: карты остались у места 1, остальные вышли. */
    private static DealStateFixture finishedDeal() {
        return aDeal()
                .withPhase(DealPhase.DEAL_OVER)
                .withEmptyDeck()
                .withPlayerOutOfDeal(0)
                .withPlayerOutOfDeal(2);
    }
}
