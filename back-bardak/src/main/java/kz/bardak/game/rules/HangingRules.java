package kz.bardak.game.rules;

import java.util.ArrayList;
import java.util.List;
import java.util.Objects;

/**
 * Правила навеса — центральной механики бардака (§2.3).
 *
 * <p>Навесить = положить карту из своей руки в чужой слот. Выгода двойная: избавляешься от
 * карты и продвигаешь соперника к джокеру. Поэтому навес — возможность, а не обязанность.
 *
 * <p>Право навесить устроено тремя разными способами, и какой из них включится, решает
 * не роль игрока, а положение жертвы:
 * <ol>
 *   <li><b>джокер</b> — право сразу у всех, приоритета нет вообще;</li>
 *   <li><b>уникальный отстающий</b> — право у всех, и навешивают все сразу;</li>
 *   <li><b>обычный случай</b> — очередь: атаковавший → поддержавший → остальные наравне.</li>
 * </ol>
 */
public final class HangingRules {

    private final RulesConfig config;

    public HangingRules(final RulesConfig config) {
        this.config = Objects.requireNonNull(config, "config");
    }

    /** Подходит ли карта под текущую ступень жертвы. Масть не важна. */
    public boolean isFlyingCard(final PlayerState victim, final Card card) {
        return config.navesScale().isFlyingCard(victim.navesLevel(), card);
    }

    /** Следующая ступень — джокер: право навесить сразу у всех (§2.3). */
    public boolean nextIsJoker(final PlayerState victim) {
        return config.navesScale().nextIsJoker(victim.navesLevel());
    }

    /**
     * ⭐ Отстающий: у жертвы самый низкий уровень среди оставшихся в раздаче, и такой она
     * <b>одна</b>. Разделённый минимум правило не включает — навес тогда обычный (ADR-028).
     *
     * <p>Вышедшие в сравнении не участвуют, даже если их уровень ниже всех.
     */
    public boolean isUniqueLaggard(final DealState state, final int victimSeat) {
        final int victimLevel = state.playerAt(victimSeat).navesLevel();
        return state.players().stream()
                .filter(PlayerState::inDeal)
                .filter(player -> player.seatNo() != victimSeat)
                .allMatch(player -> player.navesLevel() > victimLevel);
    }

    /**
     * Право равно у всех: навешивают либо джокер, либо уникальному отстающему. В обоих
     * случаях приоритет не действует.
     */
    public boolean isRightEqualForAll(final DealState state, final int victimSeat) {
        return nextIsJoker(state.playerAt(victimSeat)) || isUniqueLaggard(state, victimSeat);
    }

    /**
     * ⭐ Отстающего добивают всем столом: карты в слот уходят от каждого желающего, а уровень
     * поднимается ровно на одну ступень. Джокер так не работает — он один и решает исход.
     */
    public boolean isEveryClaimantHanging(final DealState state, final int victimSeat) {
        return !nextIsJoker(state.playerAt(victimSeat)) && isUniqueLaggard(state, victimSeat);
    }

    /**
     * Очередь права в обычном случае (§2.3): атаковавший — начавший раунд, затем
     * поддержавший — второй сосед, затем все остальные наравне.
     */
    public List<Integer> priorityOrder(final DealState state, final int victimSeat) {
        final List<Integer> order = new ArrayList<>();
        addCandidate(state, order, victimSeat, state.roundStarterSeat());
        addCandidate(state, order, victimSeat, state.nextActiveSeatAfter(victimSeat));
        for (int step = 1; step <= state.players().size(); step++) {
            addCandidate(state, order, victimSeat, (state.roundStarterSeat() + step) % state.players().size());
        }
        return List.copyOf(order);
    }

    /** Кто вообще способен навесить: держит нужную карту и это не сама жертва. */
    public List<Integer> seatsHoldingFlyingCard(final DealState state, final int victimSeat) {
        final PlayerState victim = state.playerAt(victimSeat);
        return priorityOrder(state, victimSeat).stream()
                .filter(seat -> state.playerAt(seat).hand().stream().anyMatch(card -> isFlyingCard(victim, card)))
                .toList();
    }

    /**
     * Можно ли навесить эту карту. Проверка не смотрит на очередь — очередь ведёт автомат,
     * а здесь только то, что зависит от карт и от положения игроков.
     */
    public MoveVerdict canHang(final DealState state, final int seatNo, final int victimSeat, final Card card) {
        Objects.requireNonNull(state, "state");
        Objects.requireNonNull(card, "card");
        if (!config.navesEnabled()) {
            return MoveVerdict.rejected(RejectionReason.NAVES_DISABLED);
        }
        if (seatNo == victimSeat) {
            return MoveVerdict.rejected(RejectionReason.CANNOT_HANG_ON_SELF);
        }
        if (!state.playerAt(seatNo).holdsInHand(card)) {
            return MoveVerdict.rejected(RejectionReason.CARD_NOT_IN_HAND);
        }
        if (!isFlyingCard(state.playerAt(victimSeat), card)) {
            return MoveVerdict.rejected(RejectionReason.CARD_NOT_ON_NAVES_SCALE);
        }
        return MoveVerdict.allowed();
    }

    private void addCandidate(final DealState state, final List<Integer> order,
                              final int victimSeat, final int seat) {
        if (seat != victimSeat && state.playerAt(seat).inDeal() && !order.contains(seat)) {
            order.add(seat);
        }
    }
}
