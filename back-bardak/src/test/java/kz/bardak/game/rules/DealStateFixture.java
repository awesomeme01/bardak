package kz.bardak.game.rules;

import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;

/**
 * Сборка снимка раздачи для тестов. Нужна затем, чтобы в самом тесте оставалось только то,
 * что он проверяет, — остальное берётся из осмысленных умолчаний: трое игроков, козырь
 * черви, колода не пуста, стол пуст, атакует место 0, отбивается место 1.
 */
final class DealStateFixture {

    private DealPhase phase = DealPhase.ATTACK;
    private Trump trump = Trump.of(Suit.HEARTS);
    private List<Card> deck = List.of(PipCard.of(Rank.SIX, Suit.CLUBS));
    private List<PlayerState> players = List.of(
            PlayerState.of(0, List.of(), null),
            PlayerState.of(1, List.of(), null),
            PlayerState.of(2, List.of(), null));
    private List<TableSlot> table = List.of();
    private int attackRightSeat = 0;
    private int defenderSeat = 1;
    private boolean anyCardBeatenThisRound;
    private boolean anyPileDiscarded;

    static DealStateFixture aDeal() {
        return new DealStateFixture();
    }

    /** Карты в руке игрока на указанном месте; остальные поля игрока не трогаются. */
    DealStateFixture withHand(final int seatNo, final Card... cards) {
        return withPlayer(seatNo, PlayerState.of(seatNo, Arrays.asList(cards), player(seatNo).faceDownCard()));
    }

    DealStateFixture withFaceDownCard(final int seatNo, final Card card) {
        return withPlayer(seatNo, PlayerState.of(seatNo, player(seatNo).hand(), card));
    }

    DealStateFixture withPlayerOutOfDeal(final int seatNo) {
        final PlayerState player = player(seatNo);
        return withPlayer(seatNo,
                new PlayerState(seatNo, player.hand(), player.faceDownCard(), false));
    }

    DealStateFixture withEmptyDeck() {
        this.deck = List.of();
        return this;
    }

    DealStateFixture withDeckOf(final int cardCount) {
        final List<Card> cards = new ArrayList<>();
        for (int index = 0; index < cardCount; index++) {
            cards.add(new JokerCard(index + 1));
        }
        this.deck = List.copyOf(cards);
        return this;
    }

    /** Неотбитые атакующие карты на столе. */
    DealStateFixture withAttackCards(final Card... cards) {
        final List<TableSlot> slots = new ArrayList<>(table);
        for (final Card card : cards) {
            slots.add(TableSlot.of(card));
        }
        this.table = List.copyOf(slots);
        return this;
    }

    /** Пара «атака — чем бита»; заодно включает флаг «в раунде уже отбивались». */
    DealStateFixture withBeatenPair(final Card attack, final Card defence) {
        final List<TableSlot> slots = new ArrayList<>(table);
        slots.add(TableSlot.of(attack).beatenWith(defence));
        this.table = List.copyOf(slots);
        this.anyCardBeatenThisRound = true;
        return this;
    }

    DealStateFixture withAnyPileDiscarded() {
        this.anyPileDiscarded = true;
        return this;
    }

    DealStateFixture withAttackRightAt(final int seatNo) {
        this.attackRightSeat = seatNo;
        return this;
    }

    DealStateFixture withDefenderAt(final int seatNo) {
        this.defenderSeat = seatNo;
        return this;
    }

    DealStateFixture withTrump(final Suit suit) {
        this.trump = Trump.of(suit);
        return this;
    }

    DealStateFixture withPhase(final DealPhase phase) {
        this.phase = phase;
        return this;
    }

    DealState build() {
        return new DealState(phase, trump, deck, players, table,
                attackRightSeat, defenderSeat, anyCardBeatenThisRound, anyPileDiscarded);
    }

    private PlayerState player(final int seatNo) {
        return players.get(seatNo);
    }

    private DealStateFixture withPlayer(final int seatNo, final PlayerState player) {
        final List<PlayerState> updated = new ArrayList<>(players);
        updated.set(seatNo, player);
        this.players = List.copyOf(updated);
        return this;
    }
}
