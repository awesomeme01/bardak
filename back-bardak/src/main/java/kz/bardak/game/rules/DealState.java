package kz.bardak.game.rules;

import java.util.ArrayList;
import java.util.List;
import java.util.Objects;
import java.util.Optional;

/**
 * Снимок раздачи — всё, что нужно правилам, и ничего больше. Неизменяемый: движок работает
 * как {@code apply(state, command) -> (newState, events)}.
 *
 * @param phase              фаза автомата раздачи (§4.2)
 * @param trump              козырь и вытекающая защищённая масть (§1.1.1). {@code null}
 *                           только в фазах {@link DealPhase#DEALING} и
 *                           {@link DealPhase#DICE}: нижней картой оказался джокер, и масть
 *                           ещё не названа (§1.2)
 * @param deck               остаток колоды: индекс 0 — верх, последняя карта — потайной
 *                           козырь (§1.9)
 * @param players            игроки по местам; индекс списка равен {@code seatNo}
 * @param table              стол как список пар «атака — чем бита» (§2.1)
 * @param roundStarterSeat   кто начал текущий раунд — от него считается порядок подкида
 *                           и порядок добора (§2.1, §1.4.1)
 * @param attackRightSeat    у кого сейчас право положить атакующую карту (§2.1)
 * @param defenderSeat       кто отбивается
 * @param passedSeats        кто уже спасовал в этом раунде: право назад не возвращается
 * @param exitOrder          места в порядке выхода из раздачи — первым вышедший первым
 *                           (нужно для сдвигов шкалы, §0.1)
 * @param anyCardBeatenThisRound хотя бы одна карта в раунде отбита — выключатель перевода
 *                               (§2.2, ADR-031)
 * @param anyPileDiscarded   в этой раздаче уже уходили карты в отбой — от этого зависит
 *                           потолок атаки, а не от стола (§1.5, ADR-023)
 * @param hangingWindow      открытое окно навеса или {@code null}, если окна нет (§2.3)
 * @param lastAttackCards    состав последней атаки раздачи. ⭐ Нужен для степеней проигрыша:
 *                           считаются восьмёрки в том, что было выложено, а не в том, что
 *                           попало игроку в руку (§0.3)
 * @param pendingHiddenTrump потайной козырь-джокер, ждущий выбора масти (§1.9); обычно
 *                           {@code null}
 * @param rngSeed            под-seed раздачи: всё случайное выводится из него (§6)
 * @param diceRolls          сколько бросков кости уже случилось в раздаче — чтобы два спора
 *                           подряд не давали одинаковый результат
 */
public record DealState(
        DealPhase phase,
        Trump trump,
        List<Card> deck,
        List<PlayerState> players,
        List<TableSlot> table,
        int roundStarterSeat,
        int attackRightSeat,
        int defenderSeat,
        List<Integer> passedSeats,
        List<Integer> exitOrder,
        boolean anyCardBeatenThisRound,
        boolean anyPileDiscarded,
        HangingWindow hangingWindow,
        List<Card> lastAttackCards,
        PendingHiddenTrump pendingHiddenTrump,
        long rngSeed,
        int diceRolls) {

    public DealState {
        Objects.requireNonNull(phase, "phase");
        deck = List.copyOf(Objects.requireNonNull(deck, "deck"));
        players = List.copyOf(Objects.requireNonNull(players, "players"));
        table = List.copyOf(Objects.requireNonNull(table, "table"));
        passedSeats = List.copyOf(Objects.requireNonNull(passedSeats, "passedSeats"));
        exitOrder = List.copyOf(Objects.requireNonNull(exitOrder, "exitOrder"));
        lastAttackCards = List.copyOf(Objects.requireNonNull(lastAttackCards, "lastAttackCards"));
    }

    /** Козырь назван. В фазе выбора масти его ещё нет (§1.2). */
    public boolean hasTrump() {
        return trump != null;
    }

    public boolean isDeckEmpty() {
        return deck.isEmpty();
    }

    public PlayerState playerAt(final int seatNo) {
        if (seatNo < 0 || seatNo >= players.size()) {
            throw new IllegalArgumentException("За столом нет места " + seatNo);
        }
        return players.get(seatNo);
    }

    public PlayerState defender() {
        return playerAt(defenderSeat);
    }

    /** Сколько атакующих карт уже лежит на столе — с ними сверяется потолок атаки. */
    public int attackCardCount() {
        return table.size();
    }

    /** Сколько атакующих карт ещё не отбито — с ними сверяется рука защищающегося. */
    public int unbeatenCount() {
        return (int) table.stream().filter(slot -> !slot.isBeaten()).count();
    }

    /**
     * Есть ли на столе карта такого же ранга — условие подкидывания (§1.4). Считаются
     * и атакующие карты, и те, которыми отбивались.
     */
    public boolean hasRankOnTable(final Card card) {
        Objects.requireNonNull(card, "card");
        return table.stream().anyMatch(slot -> slot.attack().sameRankAs(card)
                || slot.defenceCard().filter(card::sameRankAs).isPresent());
    }

    /** Все карты со стола — то, что забирает взявший (§1.4). */
    public List<Card> tableCards() {
        final List<Card> cards = new ArrayList<>();
        for (final TableSlot slot : table) {
            cards.add(slot.attack());
            slot.defenceCard().ifPresent(cards::add);
        }
        return List.copyOf(cards);
    }

    /**
     * Следующее по часовой стрелке место, где ещё есть игрок в раздаче. Именно к нему
     * уезжает защита при переводе (§2.2) и переходит ход после «взял».
     */
    public int nextActiveSeatAfter(final int seatNo) {
        for (int step = 1; step <= players.size(); step++) {
            final int candidate = (seatNo + step) % players.size();
            if (playerAt(candidate).inDeal()) {
                return candidate;
            }
        }
        throw new IllegalStateException("В раздаче не осталось игроков после места " + seatNo);
    }

    public long playersInDeal() {
        return players.stream().filter(PlayerState::inDeal).count();
    }

    public boolean hasPassed(final int seatNo) {
        return passedSeats.contains(seatNo);
    }

    /** Потайной козырь-джокер ждёт, пока назовут масть (§1.9). */
    public Optional<PendingHiddenTrump> hiddenTrumpAwaitingSuit() {
        return Optional.ofNullable(pendingHiddenTrump);
    }

    public Optional<HangingWindow> hanging() {
        return Optional.ofNullable(hangingWindow);
    }

    public Builder toBuilder() {
        return new Builder(this);
    }

    /**
     * Точечное изменение снимка. Нужен потому, что запись из пятнадцати частей неудобно
     * пересобирать целиком ради одного изменившегося поля.
     */
    public static final class Builder {

        private DealPhase phase;
        private Trump trump;
        private List<Card> deck;
        private List<PlayerState> players;
        private List<TableSlot> table;
        private int roundStarterSeat;
        private int attackRightSeat;
        private int defenderSeat;
        private List<Integer> passedSeats;
        private List<Integer> exitOrder;
        private boolean anyCardBeatenThisRound;
        private boolean anyPileDiscarded;
        private HangingWindow hangingWindow;
        private List<Card> lastAttackCards;
        private PendingHiddenTrump pendingHiddenTrump;
        private long rngSeed;
        private int diceRolls;

        private Builder(final DealState state) {
            this.phase = state.phase;
            this.trump = state.trump;
            this.deck = state.deck;
            this.players = state.players;
            this.table = state.table;
            this.roundStarterSeat = state.roundStarterSeat;
            this.attackRightSeat = state.attackRightSeat;
            this.defenderSeat = state.defenderSeat;
            this.passedSeats = state.passedSeats;
            this.exitOrder = state.exitOrder;
            this.anyCardBeatenThisRound = state.anyCardBeatenThisRound;
            this.anyPileDiscarded = state.anyPileDiscarded;
            this.hangingWindow = state.hangingWindow;
            this.lastAttackCards = state.lastAttackCards;
            this.pendingHiddenTrump = state.pendingHiddenTrump;
            this.rngSeed = state.rngSeed;
            this.diceRolls = state.diceRolls;
        }

        public Builder phase(final DealPhase value) {
            this.phase = value;
            return this;
        }

        public Builder trump(final Trump value) {
            this.trump = value;
            return this;
        }

        public Builder deck(final List<Card> value) {
            this.deck = value;
            return this;
        }

        public Builder players(final List<PlayerState> value) {
            this.players = value;
            return this;
        }

        public Builder player(final PlayerState value) {
            final List<PlayerState> updated = new ArrayList<>(players);
            updated.set(value.seatNo(), value);
            this.players = List.copyOf(updated);
            return this;
        }

        public Builder table(final List<TableSlot> value) {
            this.table = value;
            return this;
        }

        public Builder roundStarterSeat(final int value) {
            this.roundStarterSeat = value;
            return this;
        }

        public Builder attackRightSeat(final int value) {
            this.attackRightSeat = value;
            return this;
        }

        public Builder defenderSeat(final int value) {
            this.defenderSeat = value;
            return this;
        }

        public Builder passedSeats(final List<Integer> value) {
            this.passedSeats = value;
            return this;
        }

        public Builder exitOrder(final List<Integer> value) {
            this.exitOrder = value;
            return this;
        }

        public Builder anyCardBeatenThisRound(final boolean value) {
            this.anyCardBeatenThisRound = value;
            return this;
        }

        public Builder anyPileDiscarded(final boolean value) {
            this.anyPileDiscarded = value;
            return this;
        }

        public Builder hangingWindow(final HangingWindow value) {
            this.hangingWindow = value;
            return this;
        }

        public Builder lastAttackCards(final List<Card> value) {
            this.lastAttackCards = value;
            return this;
        }

        public Builder pendingHiddenTrump(final PendingHiddenTrump value) {
            this.pendingHiddenTrump = value;
            return this;
        }

        public Builder rngSeed(final long value) {
            this.rngSeed = value;
            return this;
        }

        public Builder diceRolls(final int value) {
            this.diceRolls = value;
            return this;
        }

        public DealState build() {
            return new DealState(phase, trump, deck, players, table, roundStarterSeat,
                    attackRightSeat, defenderSeat, passedSeats, exitOrder,
                    anyCardBeatenThisRound, anyPileDiscarded, hangingWindow, lastAttackCards,
                    pendingHiddenTrump, rngSeed, diceRolls);
        }
    }
}
