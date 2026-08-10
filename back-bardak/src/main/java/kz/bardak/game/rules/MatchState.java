package kz.bardak.game.rules;

import java.util.List;
import java.util.Objects;
import java.util.Optional;

/**
 * Состояние матча — «бардака» целиком (§0).
 *
 * <p>⭐ Матч первичен, раздача вложена в него. Разделение не косметическое: уровни навесов
 * живут здесь и переживают перераздачу, а руки, слоты и колода живут в {@link DealState}
 * и обнуляются вместе с ней (ADR-018, §0.6).
 *
 * @param phase       идёт раздача или матч уже закончен
 * @param navesLevels уровни по местам — счёт матча (ADR-017)
 * @param dealNo      номер текущей раздачи, с единицы
 * @param matchSeed   seed матча; под-seed каждой раздачи производится от него (§6)
 * @param deal        текущая раздача; после конца матча — последняя сыгранная
 * @param results     итоги сыгранных раздач по порядку
 */
public record MatchState(
        MatchPhase phase,
        List<Integer> navesLevels,
        int dealNo,
        long matchSeed,
        DealState deal,
        List<DealOutcome> results) {

    public MatchState {
        Objects.requireNonNull(phase, "phase");
        navesLevels = List.copyOf(Objects.requireNonNull(navesLevels, "navesLevels"));
        Objects.requireNonNull(deal, "deal");
        results = List.copyOf(Objects.requireNonNull(results, "results"));
    }

    public int playerCount() {
        return navesLevels.size();
    }

    public int navesLevelAt(final int seatNo) {
        return navesLevels.get(seatNo);
    }

    public boolean isOver() {
        return phase == MatchPhase.MATCH_OVER;
    }

    /** Итог последней раздачи — в нём же и проигравшие матч, если матч закончился. */
    public Optional<DealOutcome> lastResult() {
        return results.isEmpty() ? Optional.empty() : Optional.of(results.get(results.size() - 1));
    }

    /** Главный проигравший матча (§0.3). Пусто, пока матч идёт. */
    public Optional<PlayerOutcome> mainLoser() {
        return lastResult().flatMap(DealOutcome::mainLoser);
    }

    public Builder toBuilder() {
        return new Builder(this);
    }

    /** Точечное изменение снимка матча. */
    public static final class Builder {

        private MatchPhase phase;
        private List<Integer> navesLevels;
        private int dealNo;
        private final long matchSeed;
        private DealState deal;
        private List<DealOutcome> results;

        private Builder(final MatchState state) {
            this.phase = state.phase;
            this.navesLevels = state.navesLevels;
            this.dealNo = state.dealNo;
            this.matchSeed = state.matchSeed;
            this.deal = state.deal;
            this.results = state.results;
        }

        public Builder phase(final MatchPhase value) {
            this.phase = value;
            return this;
        }

        public Builder navesLevels(final List<Integer> value) {
            this.navesLevels = value;
            return this;
        }

        public Builder dealNo(final int value) {
            this.dealNo = value;
            return this;
        }

        public Builder deal(final DealState value) {
            this.deal = value;
            return this;
        }

        public Builder results(final List<DealOutcome> value) {
            this.results = value;
            return this;
        }

        public MatchState build() {
            return new MatchState(phase, navesLevels, dealNo, matchSeed, deal, results);
        }
    }
}
