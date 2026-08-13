package kz.bardak.game.rules;

import java.util.ArrayList;
import java.util.List;
import java.util.Objects;

/**
 * Персональная проекция состояния (fog of war) и фильтр событий.
 *
 * <p>⭐ Проекция строит {@link PlayerView} <b>из своих карт и общих данных</b>, а не
 * фильтрует полное состояние. Разница принципиальная: отфильтровать можно забыть, а собрать
 * из того, чего нет, — нельзя.
 *
 * <p>Список доступных действий считается <b>перебором через сам движок</b>: команда попадает
 * в список, только если движок её принял на копии состояния. Поэтому список не может
 * разойтись с правилами — он и есть правила.
 */
public final class StateProjection {

    private final RulesConfig config;
    private final DealEngine engine;

    public StateProjection(final RulesConfig config, final DealEngine engine) {
        this.config = Objects.requireNonNull(config, "config");
        this.engine = Objects.requireNonNull(engine, "engine");
    }

    public static StateProjection withDefaults() {
        return new StateProjection(RulesConfig.defaults(), DealEngine.withDefaults());
    }

    public PlayerView project(final DealState state, final int viewerSeat) {
        Objects.requireNonNull(state, "state");
        final PlayerState viewer = state.playerAt(viewerSeat);
        return new PlayerView(
                viewerSeat,
                state.phase(),
                state.hasTrump() ? state.trump().suit() : null,
                state.hasTrump() ? state.trump().protectedSuit() : null,
                state.deck().size(),
                discardCount(state),
                viewer.hand(),
                viewer.hasFaceDownCard(),
                state.table(),
                seats(state),
                state.roundStarterSeat(),
                state.attackRightSeat(),
                state.defenderSeat(),
                state.hanging().map(HangingWindow::victimSeat).orElse(null),
                availableActions(state, viewerSeat));
    }

    /**
     * Сколько карт ушло в отбой.
     *
     * <p>Прямого счётчика в раздаче нет — отбитые карты просто исчезают со стола. Считаем
     * от обратного: всё, чего нет ни в колоде, ни на руках, ни на столе, ни в навесах,
     * лежит в отбое.
     *
     * <p>⚠️ Счёт верен для состояний, сданных {@link Dealer}: он держится на том, что все
     * карты колоды где-то есть. Снимок, собранный вручную (в тестах), даст ерунду — и это
     * не поломка, а цена того, что отдельного счётчика не заводится.
     */
    private int discardCount(final DealState state) {
        int inPlay = state.deck().size();
        for (final PlayerState player : state.players()) {
            inPlay += player.hand().size() + player.hungCards().size();
            if (player.hasFaceDownCard()) {
                inPlay++;
            }
        }
        for (final TableSlot slot : state.table()) {
            inPlay += slot.defenceCard().isPresent() ? 2 : 1;
        }
        return Math.max(0, new DeckFactory().buildOrdered(state.players().size()).size() - inPlay);
    }

    /**
     * События, которые вправе увидеть этот игрок.
     *
     * <p>⭐ Вскрытие скрытой карты видит <b>только владелец</b>: карта переходит в его руку
     * и дальше он играет ею как обычной, а чужая рука никому не показывается (§1.8).
     * Остальные узнают лишь то, что видно в проекции: скрытой карты у него больше нет,
     * а карт в руке стало на одну больше.
     */
    public List<DealEvent> eventsFor(final List<DealEvent> events, final int viewerSeat) {
        Objects.requireNonNull(events, "events");
        return events.stream()
                .filter(event -> event.privateToSeat().map(seat -> seat == viewerSeat).orElse(true))
                .toList();
    }

    private List<SeatView> seats(final DealState state) {
        final List<SeatView> seats = new ArrayList<>();
        for (final PlayerState player : state.players()) {
            seats.add(new SeatView(
                    player.seatNo(),
                    player.handSize(),
                    player.hasFaceDownCard(),
                    player.hungCards(),
                    player.navesLevel(),
                    config.navesScale().nextRank(player.navesLevel()).orElse(null),
                    config.navesScale().nextIsJoker(player.navesLevel()),
                    state.hasPassed(player.seatNo()),
                    player.inDeal()));
        }
        return List.copyOf(seats);
    }

    /**
     * Что игрок может сделать прямо сейчас. Кандидаты собираются только из его собственных
     * карт и того, что лежит на столе, — чужие карты в перебор не попадают даже мельком.
     */
    private List<DealCommand> availableActions(final DealState state, final int seat) {
        final List<DealCommand> accepted = new ArrayList<>();
        for (final DealCommand candidate : candidates(state, seat)) {
            if (engine.apply(state, candidate).isApplied()) {
                accepted.add(candidate);
            }
        }
        return List.copyOf(accepted);
    }

    private List<DealCommand> candidates(final DealState state, final int seat) {
        final List<DealCommand> candidates = new ArrayList<>();
        final PlayerState viewer = state.playerAt(seat);
        for (final Suit suit : Suit.values()) {
            candidates.add(new DealCommand.ChooseTrump(seat, suit));
        }
        for (final Card card : viewer.hand()) {
            candidates.add(new DealCommand.Attack(seat, card));
            candidates.add(new DealCommand.Transfer(seat, card));
            candidates.add(new DealCommand.HangCard(seat, card));
            for (final TableSlot slot : state.table()) {
                candidates.add(new DealCommand.Defend(seat, card, slot.attack()));
            }
        }
        candidates.add(new DealCommand.RevealFaceDown(seat));
        for (final TableSlot slot : state.table()) {
            candidates.add(new DealCommand.RevealFaceDownToDefend(seat, slot.attack()));
        }
        candidates.add(new DealCommand.Take(seat));
        candidates.add(new DealCommand.Pass(seat));
        candidates.add(new DealCommand.HangSkip(seat));
        return candidates;
    }
}
