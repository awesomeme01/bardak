package kz.bardak.game.rules;

import java.util.List;
import java.util.Objects;

/**
 * Снимок раздачи — всё, что нужно правилам, и ничего больше. Неизменяемый: движок работает
 * как {@code apply(state, command) -> (newState, events)}.
 *
 * @param phase              фаза автомата раздачи (§4.2)
 * @param trump              козырь и вытекающая защищённая масть (§1.1.1)
 * @param deck               остаток колоды: индекс 0 — верх, последняя карта — потайной
 *                           козырь (§1.9)
 * @param players            игроки по местам; индекс списка равен {@code seatNo}
 * @param table              стол как список пар «атака — чем бита» (§2.1)
 * @param attackRightSeat    у кого сейчас право положить атакующую карту (§2.1)
 * @param defenderSeat       кто отбивается
 * @param anyCardBeatenThisRound хотя бы одна карта в раунде отбита — выключатель перевода
 *                               (§2.2, ADR-031)
 * @param anyPileDiscarded   в этой раздаче уже уходили карты в отбой — от этого зависит
 *                           потолок атаки, а не от стола (§1.5, ADR-023)
 */
public record DealState(
        DealPhase phase,
        Trump trump,
        List<Card> deck,
        List<PlayerState> players,
        List<TableSlot> table,
        int attackRightSeat,
        int defenderSeat,
        boolean anyCardBeatenThisRound,
        boolean anyPileDiscarded) {

    public DealState {
        Objects.requireNonNull(phase, "phase");
        Objects.requireNonNull(trump, "trump");
        deck = List.copyOf(Objects.requireNonNull(deck, "deck"));
        players = List.copyOf(Objects.requireNonNull(players, "players"));
        table = List.copyOf(Objects.requireNonNull(table, "table"));
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

    /**
     * Следующее по часовой стрелке место, где ещё есть игрок в раздаче. Именно к нему
     * уезжает защита при переводе (§2.2).
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
}
