package kz.bardak.game.rules;

import java.util.Objects;

/**
 * Игровые числа стола. Явное требование §1.6: движок не опирается на конкретные числа —
 * в коде не должно быть литералов {@code 6}, {@code 5}, {@code 30}. Всё живёт здесь,
 * попадает в {@code rules_snapshot} матча и параметризует тесты.
 *
 * @param dealSize            сколько карт на руках после добора
 * @param maxAttackFirstRound потолок атакующих карт, пока в отбое пусто (§1.5)
 * @param maxAttackPerRound   потолок атакующих карт после первой биты — он же потолок защиты
 * @param transfersEnabled    разрешены ли переводы (§2.2)
 * @param jokersEnabled       участвуют ли джокеры в колоде
 * @param navesEnabled        включена ли центральная механика — навесы (§2.3)
 * @param navesScale          длина личной шкалы; укоротить её должно быть правкой конфига
 */
public record RulesConfig(
        int dealSize,
        int maxAttackFirstRound,
        int maxAttackPerRound,
        boolean transfersEnabled,
        boolean jokersEnabled,
        boolean navesEnabled,
        NavesScale navesScale) {

    public RulesConfig {
        requirePositive(dealSize, "dealSize");
        requirePositive(maxAttackFirstRound, "maxAttackFirstRound");
        requirePositive(maxAttackPerRound, "maxAttackPerRound");
        Objects.requireNonNull(navesScale, "navesScale");
    }

    /** Значения по умолчанию из §1.6 — стартовая точка стола, не константы движка. */
    public static RulesConfig defaults() {
        return new RulesConfig(6, 5, 6, true, true, true, NavesScale.full());
    }

    /** Каркас без навесов: серия раздач без продвижения по шкале, годится только для отладки. */
    public RulesConfig withoutNaves() {
        return new RulesConfig(dealSize, maxAttackFirstRound, maxAttackPerRound,
                transfersEnabled, jokersEnabled, false, navesScale);
    }

    public RulesConfig withTransfersDisabled() {
        return new RulesConfig(dealSize, maxAttackFirstRound, maxAttackPerRound,
                false, jokersEnabled, navesEnabled, navesScale);
    }

    /**
     * Потолок атакующих карт в текущем раунде. Зависит не от того, что лежит на столе,
     * а от того, уходили ли уже карты в отбой в этой раздаче (§1.5, ADR-023).
     */
    public int attackLimit(final boolean anyPileDiscarded) {
        return anyPileDiscarded ? maxAttackPerRound : maxAttackFirstRound;
    }

    private static void requirePositive(final int value, final String name) {
        if (value <= 0) {
            throw new IllegalArgumentException("%s должен быть положительным, получено: %d"
                    .formatted(Objects.requireNonNull(name, "name"), value));
        }
    }
}
