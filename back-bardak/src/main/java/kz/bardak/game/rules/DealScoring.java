package kz.bardak.game.rules;

import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;

/**
 * Подсчёт итога раздачи (§0.1, §0.3, §0.4) — самая коварная часть правил.
 *
 * <p>⭐ Считать поигроково и независимо нельзя: судьба навесившего зависит от судьбы того,
 * кому он навесил (corner case 3). Поэтому проходов четыре, и порядок между ними —
 * это и есть правило, а не деталь реализации:
 *
 * <ol>
 *   <li>автоматические сдвиги: {@code +1} проигравшему раздачу, {@code −1} вышедшему первым;</li>
 *   <li>кто проиграл игру — джокер <b>и</b> не вышел первым;</li>
 *   <li>{@code −1} каждому, кто добил проигравшего джокером; награды суммируются;</li>
 *   <li>нижняя граница и степени проигрыша.</li>
 * </ol>
 *
 * <p>⭐ Нижняя граница применяется <b>в конце</b>, а не после каждого сдвига: иначе игрок,
 * получивший {@code +1} и {@code −2}, упёрся бы в «летит 6» раньше времени и пришёл бы
 * не туда.
 */
public final class DealScoring {

    private final RulesConfig config;

    public DealScoring(final RulesConfig config) {
        this.config = Objects.requireNonNull(config, "config");
    }

    /**
     * Итог законченной раздачи.
     *
     * @param state состояние в фазе {@link DealPhase#DEAL_OVER}: карты остались у одного
     */
    public DealOutcome score(final DealState state) {
        Objects.requireNonNull(state, "state");
        final int dealLoser = dealLoserSeat(state);
        final Map<Integer, Integer> levels = startingLevels(state);

        applyAutomaticShifts(state, levels, dealLoser);
        final List<Integer> gameLosers = gameLosers(state, levels);
        applyFinisherRewards(state, levels, gameLosers);

        return new DealOutcome(outcomes(state, levels, dealLoser, gameLosers), dealLoser);
    }

    /** «Дурак» раздачи — единственный, у кого остались карты (§0.2). */
    private int dealLoserSeat(final DealState state) {
        return state.players().stream()
                .filter(PlayerState::inDeal)
                .mapToInt(PlayerState::seatNo)
                .findFirst()
                .orElseThrow(() -> new IllegalStateException("Раздача без проигравшего невозможна"));
    }

    private Map<Integer, Integer> startingLevels(final DealState state) {
        final Map<Integer, Integer> levels = new HashMap<>();
        for (final PlayerState player : state.players()) {
            levels.put(player.seatNo(), player.navesLevel());
        }
        return levels;
    }

    /**
     * Проход 1. Проигравший раздачу получает {@code +1}, вышедший первым — {@code −1}.
     * Сдвиги суммируются и друг друга не заменяют.
     */
    private void applyAutomaticShifts(final DealState state, final Map<Integer, Integer> levels,
                                      final int dealLoser) {
        levels.merge(dealLoser, 1, Integer::sum);
        firstOut(state).ifPresent(seat -> levels.merge(seat, -1, Integer::sum));
    }

    private java.util.Optional<Integer> firstOut(final DealState state) {
        return state.exitOrder().stream().findFirst();
    }

    /**
     * Проход 2. Проиграл игру тот, у кого джокер <b>и</b> кто не вышел первым (§0.2).
     * Выход первым уже снял джокер на проходе 1 — отдельной проверки не требуется.
     */
    private List<Integer> gameLosers(final DealState state, final Map<Integer, Integer> levels) {
        return state.players().stream()
                .map(PlayerState::seatNo)
                .filter(seat -> config.navesScale().isFinished(levels.get(seat)))
                .toList();
    }

    /**
     * Проход 3. За каждого проигравшего {@code −1} тому, кто навесил ему джокер.
     *
     * <p>⭐ Награда даётся за каждого добитого и суммируется: добил двоих — {@code −2}.
     * Работает и тогда, когда у самого добившего в навесе джокер: он получает {@code −1},
     * джокер снимается, и проигравшим он уже не считается.
     */
    private void applyFinisherRewards(final DealState state, final Map<Integer, Integer> levels,
                                      final List<Integer> gameLosers) {
        for (final int loser : gameLosers) {
            final int finisher = state.playerAt(loser).jokerHangerSeat();
            if (finisher != PlayerState.NOBODY) {
                levels.merge(finisher, -1, Integer::sum);
            }
        }
    }

    /**
     * Проход 4. Нижняя граница и степени. Проигравшие определяются <b>заново</b>: награда
     * на проходе 3 могла снять джокер с того, кто попал в список на проходе 2.
     */
    private List<PlayerOutcome> outcomes(final DealState state, final Map<Integer, Integer> levels,
                                         final int dealLoser, final List<Integer> gameLosers) {
        final List<PlayerOutcome> outcomes = new ArrayList<>();
        for (final PlayerState player : state.players()) {
            final int seat = player.seatNo();
            final int level = clampToScale(levels.get(seat));
            final boolean stillFinished = config.navesScale().isFinished(level);
            final LossDegree degree = stillFinished && gameLosers.contains(seat)
                    ? degreeFor(state, player, seat == dealLoser)
                    : null;
            outcomes.add(new PlayerOutcome(seat, player.navesLevel(), level, degree));
        }
        return List.copyOf(outcomes);
    }

    /**
     * ⭐ Границы шкалы. Нижняя описана явно — «летит 6», ступени «5» нет (§0.1). Верхняя
     * в правилах не названа, потому что джокер заканчивает матч, — но она есть: игрок,
     * которому навесили джокер посреди раздачи, доигрывает её (ADR-019) и может вдобавок
     * проиграть раздачу. Ступени выше джокера не существует, лишний {@code +1} пропадает.
     */
    private int clampToScale(final int level) {
        return Math.min(Math.max(level, NavesScale.NO_NAVES), config.navesScale().jokerLevel());
    }

    /**
     * Степень проигрыша (§0.3). Подходить может несколько условий сразу — берётся самая
     * тяжёлая, поэтому порядок проверок здесь и есть алгоритм.
     */
    private LossDegree degreeFor(final DealState state, final PlayerState player, final boolean lostTheDeal) {
        if (!lostTheDeal) {
            return LossDegree.FAIL;
        }
        final long eights = state.lastAttackCards().stream()
                .filter(PipCard.class::isInstance)
                .map(PipCard.class::cast)
                .filter(card -> card.rank() == Rank.EIGHT)
                .count();
        if (eights == Suit.values().length) {
            return LossDegree.ROYAL;
        }
        if (eights >= 1) {
            return LossDegree.SUPER_MEGA_SUCK;
        }
        if (player.jokerHangerSeat() != PlayerState.NOBODY) {
            return LossDegree.SUPER_MEGA_FAIL;
        }
        return LossDegree.SUPER_FAIL;
    }
}
