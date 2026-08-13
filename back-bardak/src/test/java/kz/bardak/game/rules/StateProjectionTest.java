package kz.bardak.game.rules;

import static kz.bardak.game.rules.DealStateFixture.aDeal;
import static org.assertj.core.api.Assertions.assertThat;

import java.util.ArrayList;
import java.util.List;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

/**
 * Персональная проекция состояния — fog of war (§1.8, §2.3, ADR-026).
 *
 * <p>Главный тест здесь не про поля, а про то, чего в проекции быть <b>не может</b>.
 */
class StateProjectionTest {

    private static final Card SEVEN_DIAMONDS = PipCard.of(Rank.SEVEN, Suit.DIAMONDS);
    private static final Card NINE_DIAMONDS = PipCard.of(Rank.NINE, Suit.DIAMONDS);
    private static final Card ACE_CLUBS = PipCard.of(Rank.ACE, Suit.CLUBS);
    private static final Card KING_CLUBS = PipCard.of(Rank.KING, Suit.CLUBS);
    private static final Card SIX_SPADES = PipCard.of(Rank.SIX, Suit.SPADES);

    private final StateProjection projection = StateProjection.withDefaults();

    @DisplayName("Should show my own hand in full When the state is projected")
    @Test
    void shouldShowMyOwnHandInFullWhenTheStateIsProjected() {
        final DealState state = aDeal().withHand(0, SEVEN_DIAMONDS, ACE_CLUBS).withHand(1, KING_CLUBS).build();

        final PlayerView view = projection.project(state, 0);

        assertThat(view.mySeat()).isZero();
        assertThat(view.myHand()).containsExactly(SEVEN_DIAMONDS, ACE_CLUBS);
    }

    @DisplayName("Should show only the card count of others When the state is projected")
    @Test
    void shouldShowOnlyTheCardCountOfOthersWhenTheStateIsProjected() {
        final DealState state = aDeal()
                .withHand(0, SEVEN_DIAMONDS)
                .withHand(1, KING_CLUBS, ACE_CLUBS, NINE_DIAMONDS)
                .build();

        final PlayerView view = projection.project(state, 0);

        assertThat(view.seat(1).cardsCount()).isEqualTo(3);
        assertThat(cardsVisibleIn(view)).doesNotContain(KING_CLUBS, ACE_CLUBS, NINE_DIAMONDS);
    }

    @DisplayName("Should never leak any hidden or foreign card When every seat looks at the same state")
    @Test
    void shouldNeverLeakAnyHiddenOrForeignCardWhenEverySeatLooksAtTheSameState() {
        final DealState state = aDeal()
                .withHand(0, SEVEN_DIAMONDS, ACE_CLUBS)
                .withHand(1, KING_CLUBS)
                .withHand(2, NINE_DIAMONDS)
                .withFaceDownCard(0, SIX_SPADES)
                .withFaceDownCard(1, PipCard.of(Rank.QUEEN, Suit.HEARTS))
                .withAttackCards(PipCard.of(Rank.TEN, Suit.CLUBS))
                .build();

        for (final PlayerState viewer : state.players()) {
            final List<Card> visible = cardsVisibleIn(projection.project(state, viewer.seatNo()));

            for (final PlayerState other : state.players()) {
                if (other.seatNo() != viewer.seatNo() && !other.hand().isEmpty()) {
                    assertThat(visible)
                            .withFailMessage("Рука места %d попала в проекцию места %d",
                                    other.seatNo(), viewer.seatNo())
                            .doesNotContainAnyElementsOf(other.hand());
                }
                other.faceDown().ifPresent(card -> assertThat(visible)
                        .withFailMessage("Скрытая карта места %d попала в проекцию места %d",
                                other.seatNo(), viewer.seatNo())
                        .doesNotContain(card));
            }
        }
    }

    @DisplayName("Should hide the deck contents When the state is projected")
    @Test
    void shouldHideTheDeckContentsWhenTheStateIsProjected() {
        final DealState state = aDeal().withDeckOf(5).withHand(0, SEVEN_DIAMONDS).build();

        final PlayerView view = projection.project(state, 0);

        assertThat(view.deckLeft()).isEqualTo(5);
        assertThat(cardsVisibleIn(view)).containsExactly(SEVEN_DIAMONDS);
    }

    @DisplayName("Should tell the owner only the fact of the hidden card When the state is projected")
    @Test
    void shouldTellTheOwnerOnlyTheFactOfTheHiddenCardWhenTheStateIsProjected() {
        final DealState state = aDeal()
                .withHand(0, SEVEN_DIAMONDS)
                .withFaceDownCard(0, SIX_SPADES)
                .build();

        final PlayerView view = projection.project(state, 0);

        assertThat(view.iHaveHiddenCard()).isTrue();
        assertThat(view.seat(0).hasHiddenCard()).isTrue();
        assertThat(view.myHand()).doesNotContain(SIX_SPADES);
    }

    @DisplayName("Should show the naves slot and level of everyone When the state is projected")
    @Test
    void shouldShowTheNavesSlotAndLevelOfEveryoneWhenTheStateIsProjected() {
        final DealState state = aDeal()
                .withHand(0, SEVEN_DIAMONDS)
                .withNavesLevel(1, 0)
                .build();

        final PlayerView view = projection.project(state, 0);

        assertThat(view.seat(1).navesLevel()).isZero();
        assertThat(view.seat(1).nextRank()).contains(Rank.SEVEN);
        assertThat(view.seat(1).nextIsJoker()).isFalse();
    }

    @DisplayName("Should announce the joker as the next step When the victim stands on the ace")
    @Test
    void shouldAnnounceTheJokerAsTheNextStepWhenTheVictimStandsOnTheAce() {
        final int aceLevel = NavesScale.full().jokerLevel() - 1;
        final DealState state = aDeal().withHand(0, SEVEN_DIAMONDS).withNavesLevel(1, aceLevel).build();

        final PlayerView view = projection.project(state, 0);

        assertThat(view.seat(1).nextIsJoker()).isTrue();
        assertThat(view.seat(1).nextRank()).isEmpty();
    }

    @DisplayName("Should send the protected suit alongside the trump When the trump is known")
    @Test
    void shouldSendTheProtectedSuitAlongsideTheTrumpWhenTheTrumpIsKnown() {
        final DealState state = aDeal().withTrump(Suit.SPADES).withHand(0, SEVEN_DIAMONDS).build();

        final PlayerView view = projection.project(state, 0);

        assertThat(view.trump()).contains(Suit.SPADES);
        assertThat(view.protectedSuit()).isEqualTo(Suit.CLUBS);
    }

    @DisplayName("Should leave the trump empty When it is still being rolled for")
    @Test
    void shouldLeaveTheTrumpEmptyWhenItIsStillBeingRolledFor() {
        final DealState state = new Dealer(RulesConfig.defaults(), new DiceResolver.Seeded())
                .startDeal(List.of(NavesScale.NO_NAVES, NavesScale.NO_NAVES), jokerBottomSeed());

        final PlayerView view = projection.project(state, 0);

        assertThat(view.phase()).isEqualTo(DealPhase.DICE);
        assertThat(view.trump()).isEmpty();
    }

    @DisplayName("Should offer exactly the legal moves When actions are projected")
    @Test
    void shouldOfferExactlyTheLegalMovesWhenActionsAreProjected() {
        final DealState state = aDeal()
                .withPhase(DealPhase.DEFEND)
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(1, NINE_DIAMONDS, PipCard.of(Rank.SIX, Suit.CLUBS))
                .build();

        final PlayerView view = projection.project(state, 1);

        assertThat(view.availableActions())
                .contains(new DealCommand.Defend(1, NINE_DIAMONDS, SEVEN_DIAMONDS))
                .contains(new DealCommand.Take(1))
                .doesNotContain(new DealCommand.Defend(1, PipCard.of(Rank.SIX, Suit.CLUBS), SEVEN_DIAMONDS));
    }

    @DisplayName("Should offer no action at all When it is not my turn")
    @Test
    void shouldOfferNoActionAtAllWhenItIsNotMyTurn() {
        final DealState state = aDeal()
                .withPhase(DealPhase.DEFEND)
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(2, NINE_DIAMONDS)
                .build();

        assertThat(projection.project(state, 2).availableActions()).isEmpty();
    }

    @DisplayName("Should offer the reveal without naming the card When only the hidden card is left")
    @Test
    void shouldOfferTheRevealWithoutNamingTheCardWhenOnlyTheHiddenCardIsLeft() {
        final DealState state = aDeal()
                .withEmptyDeck()
                .withFaceDownCard(0, SIX_SPADES)
                .withHand(1, ACE_CLUBS)
                .build();

        final PlayerView view = projection.project(state, 0);

        assertThat(view.availableActions()).contains(new DealCommand.RevealFaceDown(0));
        assertThat(cardsVisibleIn(view)).doesNotContain(SIX_SPADES);
    }

    @DisplayName("Should keep the reveal event to its owner When events are filtered")
    @Test
    void shouldKeepTheRevealEventToItsOwnerWhenEventsAreFiltered() {
        final List<DealEvent> events = List.of(
                new DealEvent.FaceDownRevealed(0, SIX_SPADES),
                new DealEvent.CardAttacked(0, SEVEN_DIAMONDS));

        assertThat(projection.eventsFor(events, 0)).hasSize(2);
        assertThat(projection.eventsFor(events, 1))
                .containsExactly(new DealEvent.CardAttacked(0, SEVEN_DIAMONDS));
    }

    /** Все карты, которые физически присутствуют в проекции. */
    private static List<Card> cardsVisibleIn(final PlayerView view) {
        final List<Card> cards = new ArrayList<>(view.myHand());
        for (final SeatView seat : view.seats()) {
            cards.addAll(seat.hungCards());
        }
        for (final TableSlot slot : view.table()) {
            cards.add(slot.attack());
            slot.defenceCard().ifPresent(cards::add);
        }
        for (final DealCommand action : view.availableActions()) {
            if (action instanceof DealCommand.Attack attack) {
                cards.add(attack.card());
            }
            if (action instanceof DealCommand.Defend defend) {
                cards.add(defend.card());
                cards.add(defend.target());
            }
            if (action instanceof DealCommand.HangCard hang) {
                cards.add(hang.card());
            }
            if (action instanceof DealCommand.Transfer transfer) {
                cards.add(transfer.card());
            }
        }
        return cards;
    }

    /** Первый seed, при котором нижней картой колоды оказывается джокер. */
    private static long jokerBottomSeed() {
        final Dealer dealer = new Dealer(RulesConfig.defaults(), new DiceResolver.Seeded());
        for (long seed = 1; seed < 200; seed++) {
            if (!dealer.startDeal(List.of(NavesScale.NO_NAVES, NavesScale.NO_NAVES), seed).hasTrump()) {
                return seed;
            }
        }
        throw new IllegalStateException("Не нашлось seed с джокером снизу");
    }

    @DisplayName("Should show an empty discard pile When the deal has just been dealt")
    @Test
    void shouldShowAnEmptyDiscardPileWhenTheDealHasJustBeenDealt() {
        // ⭐ Проверять надо на настоящей сдаче: счёт отбоя выводится от обратного, и его
        // правильность держится на том, что все карты колоды где-то есть. Ошибка здесь
        // тихая — «Бито 12» просто разойдётся с реальностью, и никто не заметит.
        final DealState dealt = new Dealer(RulesConfig.defaults(), new DiceResolver.Seeded())
                .startDeal(List.of(NavesScale.NO_NAVES, NavesScale.NO_NAVES), 42L);

        final PlayerView view = StateProjection.withDefaults().project(dealt, 0);

        assertThat(view.discardCount()).isZero();
    }

    @DisplayName("Should count the beaten cards When a round went to the discard pile")
    @Test
    void shouldCountTheBeatenCardsWhenARoundWentToTheDiscardPile() {
        final DealEngine engine = DealEngine.withDefaults();
        DealState state = new Dealer(RulesConfig.defaults(), new DiceResolver.Seeded())
                .startDeal(List.of(NavesScale.NO_NAVES, NavesScale.NO_NAVES), 7L);
        // Играем, пока стол не уйдёт в отбой: до этого момента отбой обязан быть пустым.
        for (int move = 0; move < 60 && StateProjection.withDefaults()
                .project(state, 0).discardCount() == 0; move++) {
            state = advance(engine, state);
        }

        assertThat(StateProjection.withDefaults().project(state, 0).discardCount()).isPositive();
    }

    /** Один ход простейшего автоигрока: первое, что примет движок. */
    private DealState advance(final DealEngine engine, final DealState state) {
        for (final DealCommand candidate : candidates(state)) {
            if (engine.apply(state, candidate) instanceof MoveResult.Applied applied) {
                return applied.state();
            }
        }
        return state;
    }

    private List<DealCommand> candidates(final DealState state) {
        final List<DealCommand> commands = new java.util.ArrayList<>();
        for (final Card card : state.playerAt(state.defenderSeat()).hand()) {
            for (final TableSlot slot : state.table()) {
                commands.add(new DealCommand.Defend(state.defenderSeat(), card, slot.attack()));
            }
        }
        for (final Card card : state.playerAt(state.attackRightSeat()).hand()) {
            commands.add(new DealCommand.Attack(state.attackRightSeat(), card));
        }
        commands.add(new DealCommand.Pass(state.attackRightSeat()));
        commands.add(new DealCommand.Take(state.defenderSeat()));
        return commands;
    }
}
