package kz.bardak.game.rules;

import java.util.ArrayList;
import java.util.List;
import java.util.Objects;
import java.util.OptionalInt;

/**
 * Автомат раздачи: {@code apply(state, command) -> (newState, events)}.
 *
 * <p>Чистая функция — ни БД, ни сети, ни Spring. Одно и то же состояние с той же командой
 * всегда даёт тот же результат, иначе невозможны ни реплей, ни воспроизведение бага (§6).
 * Отклонённая команда состояние не меняет вообще.
 */
public final class DealEngine {

    private final RulesConfig config;
    private final MoveRules moveRules;
    private final AttackOrderPolicy attackOrder;

    public DealEngine(final RulesConfig config, final AttackOrderPolicy attackOrder) {
        this.config = Objects.requireNonNull(config, "config");
        this.attackOrder = Objects.requireNonNull(attackOrder, "attackOrder");
        this.moveRules = new MoveRules(config);
    }

    public static DealEngine withDefaults() {
        return new DealEngine(RulesConfig.defaults(), new AttackOrderPolicy.BardakStrictNeighbours());
    }

    public MoveResult apply(final DealState state, final DealCommand command) {
        Objects.requireNonNull(state, "state");
        Objects.requireNonNull(command, "command");
        if (state.phase() == DealPhase.DEAL_OVER) {
            return MoveResult.rejected(RejectionReason.NOT_YOUR_TURN);
        }
        return switch (command) {
            case DealCommand.Attack attack -> applyAttack(state, attack);
            case DealCommand.Defend defend -> applyDefend(state, defend);
            case DealCommand.Transfer transfer -> applyTransfer(state, transfer);
            case DealCommand.Pass pass -> applyPass(state, pass);
            case DealCommand.Take take -> applyTake(state, take);
        };
    }

    private MoveResult applyAttack(final DealState state, final DealCommand.Attack command) {
        final MoveVerdict verdict = moveRules.canAttack(state, command.seatNo(), command.card());
        if (verdict instanceof MoveVerdict.Rejected rejected) {
            return MoveResult.rejected(rejected.reason());
        }
        final List<DealEvent> events = new ArrayList<>();
        final PlayerState player = playCard(state, command.seatNo(), command.card(), events);
        events.add(new DealEvent.CardAttacked(command.seatNo(), command.card()));

        final List<TableSlot> table = new ArrayList<>(state.table());
        table.add(TableSlot.of(command.card()));
        return MoveResult.applied(state.toBuilder()
                .player(player)
                .table(List.copyOf(table))
                .phase(DealPhase.DEFEND)
                .build(), events);
    }

    private MoveResult applyDefend(final DealState state, final DealCommand.Defend command) {
        final MoveVerdict verdict = moveRules.canDefend(state, command.seatNo(), command.card(), command.target());
        if (verdict instanceof MoveVerdict.Rejected rejected) {
            return MoveResult.rejected(rejected.reason());
        }
        final List<DealEvent> events = new ArrayList<>();
        final PlayerState player = playCard(state, command.seatNo(), command.card(), events);
        events.add(new DealEvent.CardDefended(command.seatNo(), command.card(), command.target()));

        final List<TableSlot> table = new ArrayList<>();
        for (final TableSlot slot : state.table()) {
            table.add(slot.attack().equals(command.target()) ? slot.beatenWith(command.card()) : slot);
        }
        final DealState defended = state.toBuilder()
                .player(player)
                .table(List.copyOf(table))
                .anyCardBeatenThisRound(true)
                .phase(unbeatenRemain(table) ? DealPhase.DEFEND : DealPhase.ATTACK)
                .build();
        return MoveResult.applied(defended, events);
    }

    /**
     * Перевод сдвигает защиту на следующего, а сам переводящий становится атакующим
     * (ADR-031). Роли считаются от нового защищающегося — отсюда и вытеснение прежнего
     * атакующего из раунда при четырёх и более игроках.
     */
    private MoveResult applyTransfer(final DealState state, final DealCommand.Transfer command) {
        final MoveVerdict verdict = moveRules.canTransfer(state, command.seatNo(), command.card());
        if (verdict instanceof MoveVerdict.Rejected rejected) {
            return MoveResult.rejected(rejected.reason());
        }
        final List<DealEvent> events = new ArrayList<>();
        final PlayerState player = playCard(state, command.seatNo(), command.card(), events);
        final int receiver = state.nextActiveSeatAfter(command.seatNo());
        events.add(new DealEvent.AttackTransferred(command.seatNo(), receiver, command.card()));

        final List<TableSlot> table = new ArrayList<>(state.table());
        table.add(TableSlot.of(command.card()));
        return MoveResult.applied(state.toBuilder()
                .player(player)
                .table(List.copyOf(table))
                .roundStarterSeat(command.seatNo())
                .attackRightSeat(command.seatNo())
                .defenderSeat(receiver)
                .passedSeats(List.of())
                .phase(DealPhase.DEFEND)
                .build(), events);
    }

    /**
     * «Пас» — фиксация того, что игрок больше не подкидывает. Раунд не завершается сам
     * по себе, пока обладатель права не спасовал (§2.1).
     */
    private MoveResult applyPass(final DealState state, final DealCommand.Pass command) {
        if (state.attackRightSeat() != command.seatNo() || state.hasPassed(command.seatNo())) {
            return MoveResult.rejected(RejectionReason.NOT_YOUR_TURN);
        }
        final List<Integer> passed = new ArrayList<>(state.passedSeats());
        passed.add(command.seatNo());
        final DealState afterPass = state.toBuilder().passedSeats(List.copyOf(passed)).build();

        final List<DealEvent> events = new ArrayList<>();
        events.add(new DealEvent.Passed(command.seatNo()));

        final OptionalInt next = attackOrder.nextAttacker(afterPass);
        if (next.isPresent()) {
            events.add(new DealEvent.AttackRightMoved(next.getAsInt()));
            return MoveResult.applied(afterPass.toBuilder()
                    .attackRightSeat(next.getAsInt())
                    .phase(unbeatenRemain(afterPass.table()) ? DealPhase.DEFEND : DealPhase.ATTACK)
                    .build(), events);
        }
        if (unbeatenRemain(afterPass.table())) {
            return MoveResult.applied(afterPass.toBuilder().phase(DealPhase.DEFEND).build(), events);
        }
        events.add(new DealEvent.RoundBeaten(afterPass.defenderSeat(), afterPass.tableCards()));
        return MoveResult.applied(finishRound(afterPass, false, events), events);
    }

    private MoveResult applyTake(final DealState state, final DealCommand.Take command) {
        if (state.defenderSeat() != command.seatNo()) {
            return MoveResult.rejected(RejectionReason.NOT_YOUR_TURN);
        }
        final List<Card> taken = state.tableCards();
        final List<Card> hand = new ArrayList<>(state.defender().hand());
        hand.addAll(taken);
        final PlayerState defender = new PlayerState(state.defenderSeat(), List.copyOf(hand),
                state.defender().faceDownCard(), true);

        final List<DealEvent> events = new ArrayList<>();
        events.add(new DealEvent.CardsTaken(command.seatNo(), taken));
        return MoveResult.applied(finishRound(state.toBuilder().player(defender).build(), true, events), events);
    }

    /**
     * Закрытие раунда: стол в отбой или в руку, добор в строгом порядке, выход игроков,
     * проверка конца раздачи. Порядок шагов не переставляется — добор обязан случиться
     * до проверки выхода, иначе игрок вышел бы, не получив причитающихся карт.
     *
     * <p>🟨 Навесы (§2.3) вклиниваются сюда после «взял» и появятся в M5.
     */
    private DealState finishRound(final DealState state, final boolean taken, final List<DealEvent> events) {
        final DealState cleared = state.toBuilder()
                .table(List.of())
                .anyPileDiscarded(state.anyPileDiscarded() || !taken)
                .build();
        final DealState refilled = refill(cleared, events);
        final DealState afterExits = markExits(refilled, events);
        if (afterExits.playersInDeal() <= 1) {
            final int loser = lastPlayerInDeal(afterExits);
            events.add(new DealEvent.DealFinished(loser));
            return afterExits.toBuilder().phase(DealPhase.DEAL_OVER).build();
        }
        return startNextRound(afterExits, taken);
    }

    /**
     * Добор до {@code dealSize} в порядке §1.4.1: начавший раунд → второй сосед → остальные
     * по часовой → защищавшийся последним. Порядок не формальность: колода конечна, и тот,
     * кто добирает раньше, успевает взять карты, которых не хватит остальным.
     */
    private DealState refill(final DealState state, final List<DealEvent> events) {
        final List<Card> deck = new ArrayList<>(state.deck());
        DealState current = state;
        for (final int seat : refillOrder(state)) {
            final PlayerState player = current.playerAt(seat);
            final List<Card> drawn = new ArrayList<>();
            while (player.handSize() + drawn.size() < config.dealSize() && !deck.isEmpty()) {
                drawn.add(deck.remove(0));
            }
            if (drawn.isEmpty()) {
                continue;
            }
            final List<Card> hand = new ArrayList<>(player.hand());
            hand.addAll(drawn);
            events.add(new DealEvent.CardsDrawn(seat, List.copyOf(drawn)));
            current = current.toBuilder()
                    .player(new PlayerState(seat, List.copyOf(hand), player.faceDownCard(), player.inDeal()))
                    .build();
        }
        return current.toBuilder().deck(List.copyOf(deck)).build();
    }

    private List<Integer> refillOrder(final DealState state) {
        final List<Integer> order = new ArrayList<>();
        addIfEligible(state, order, state.roundStarterSeat());
        addIfEligible(state, order, state.nextActiveSeatAfter(state.defenderSeat()));
        for (int step = 1; step <= state.players().size(); step++) {
            addIfEligible(state, order, (state.roundStarterSeat() + step) % state.players().size());
        }
        if (state.defender().inDeal()) {
            order.add(state.defenderSeat());
        }
        return order;
    }

    private void addIfEligible(final DealState state, final List<Integer> order, final int seat) {
        if (seat != state.defenderSeat() && state.playerAt(seat).inDeal() && !order.contains(seat)) {
            order.add(seat);
        }
    }

    /**
     * Выход игроков: без карт при пустой колоде, и только если нет скрытой карты (§1.7, §1.8).
     *
     * <p>⭐ Порядок проверки — тот же, что у добора, и это и есть правило «атакующий
     * опережает защищающегося» (ADR-033): карты атакующего ложатся на стол раньше
     * отбивающих, поэтому и выходит он первым. Последний оставшийся не выходит никогда —
     * он и есть «дурак» раздачи, даже если его рука тоже опустела.
     */
    private DealState markExits(final DealState state, final List<DealEvent> events) {
        final List<Integer> exitOrder = new ArrayList<>(state.exitOrder());
        DealState current = state;
        for (final int seat : refillOrder(state)) {
            if (current.playersInDeal() <= 1) {
                break;
            }
            final PlayerState player = current.playerAt(seat);
            if (!player.inDeal() || !current.isDeckEmpty() || player.handSize() > 0 || player.hasFaceDownCard()) {
                continue;
            }
            exitOrder.add(seat);
            events.add(new DealEvent.PlayerLeftDeal(seat));
            current = current.toBuilder()
                    .player(new PlayerState(seat, player.hand(), player.faceDownCard(), false))
                    .build();
        }
        return current.toBuilder().exitOrder(List.copyOf(exitOrder)).build();
    }

    /**
     * Следующий раунд: после «бито» начинает защищавшийся, после «взял» — следующий за ним
     * по кругу, а сам он раунд пропускает (§1.4).
     */
    private DealState startNextRound(final DealState state, final boolean taken) {
        final boolean defenderStillIn = state.defender().inDeal();
        final int starter = taken || !defenderStillIn
                ? state.nextActiveSeatAfter(state.defenderSeat())
                : state.defenderSeat();
        return state.toBuilder()
                .phase(DealPhase.ATTACK)
                .roundStarterSeat(starter)
                .attackRightSeat(starter)
                .defenderSeat(state.nextActiveSeatAfter(starter))
                .passedSeats(List.of())
                .anyCardBeatenThisRound(false)
                .build();
    }

    private int lastPlayerInDeal(final DealState state) {
        return state.players().stream()
                .filter(PlayerState::inDeal)
                .mapToInt(PlayerState::seatNo)
                .findFirst()
                .orElseThrow(() -> new IllegalStateException("В раздаче не осталось ни одного игрока"));
    }

    /**
     * Убирает сыгранную карту из руки — или вскрывает скрытую. Открытие необратимо (§1.8),
     * поэтому событие о вскрытии рождается здесь, а не в проверке легальности.
     */
    private PlayerState playCard(final DealState state, final int seatNo, final Card card,
                                 final List<DealEvent> events) {
        final PlayerState player = state.playerAt(seatNo);
        if (player.holdsInHand(card)) {
            final List<Card> hand = new ArrayList<>(player.hand());
            hand.remove(card);
            return new PlayerState(seatNo, List.copyOf(hand), player.faceDownCard(), player.inDeal());
        }
        events.add(new DealEvent.FaceDownRevealed(seatNo, card));
        return new PlayerState(seatNo, player.hand(), null, player.inDeal());
    }

    private boolean unbeatenRemain(final List<TableSlot> table) {
        return table.stream().anyMatch(slot -> !slot.isBeaten());
    }
}
