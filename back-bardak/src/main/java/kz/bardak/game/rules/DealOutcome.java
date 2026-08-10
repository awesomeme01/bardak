package kz.bardak.game.rules;

import java.util.Comparator;
import java.util.List;
import java.util.Objects;
import java.util.Optional;

/**
 * Итог раздачи целиком (§0.5).
 *
 * <p>⭐ Проигравших может быть несколько — это штатная ситуация, а не ошибка: несколько
 * игроков заканчивают игру с джокером и различаются степенью. Главный проигравший — тот,
 * у кого степень тяжелее.
 *
 * @param players       итоги по всем местам, в порядке мест
 * @param dealLoserSeat кто остался с картами — «дурак» раздачи
 */
public record DealOutcome(List<PlayerOutcome> players, int dealLoserSeat) {

    public DealOutcome {
        players = List.copyOf(Objects.requireNonNull(players, "players"));
    }

    public PlayerOutcome forSeat(final int seatNo) {
        return players.stream()
                .filter(outcome -> outcome.seatNo() == seatNo)
                .findFirst()
                .orElseThrow(() -> new IllegalArgumentException("За столом нет места " + seatNo));
    }

    /** Все, кто закончил игру с джокером. */
    public List<PlayerOutcome> losers() {
        return players.stream().filter(PlayerOutcome::isLoser).toList();
    }

    /** Матч окончен: кому-то навешен джокер, и он не вышел первым (§0.2). */
    public boolean isMatchOver() {
        return !losers().isEmpty();
    }

    /** Главный проигравший — по старшинству степеней (§0.3). */
    public Optional<PlayerOutcome> mainLoser() {
        return losers().stream().min(Comparator.comparing(PlayerOutcome::lossDegree));
    }
}
