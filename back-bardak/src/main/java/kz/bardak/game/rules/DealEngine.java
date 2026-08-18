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
    private final HangingRules hangingRules;
    private final AttackOrderPolicy attackOrder;
    private final DiceResolver dice;

    public DealEngine(final RulesConfig config, final AttackOrderPolicy attackOrder, final DiceResolver dice) {
        this.config = Objects.requireNonNull(config, "config");
        this.attackOrder = Objects.requireNonNull(attackOrder, "attackOrder");
        this.dice = Objects.requireNonNull(dice, "dice");
        this.moveRules = new MoveRules(config);
        this.hangingRules = new HangingRules(config);
    }

    public static DealEngine withDefaults() {
        return of(RulesConfig.defaults());
    }

    public static DealEngine of(final RulesConfig config) {
        return new DealEngine(config, new AttackOrderPolicy.BardakStrictNeighbours(), new DiceResolver.Seeded());
    }

    public MoveResult apply(final DealState state, final DealCommand command) {
        Objects.requireNonNull(state, "state");
        Objects.requireNonNull(command, "command");
        if (state.phase() == DealPhase.DEAL_OVER) {
            return MoveResult.rejected(RejectionReason.NOT_YOUR_TURN);
        }
        if (state.phase() == DealPhase.DICE && !(command instanceof DealCommand.ChooseTrump)) {
            return MoveResult.rejected(RejectionReason.TRUMP_NOT_CHOSEN_YET);
        }
        return switch (command) {
            case DealCommand.Attack attack -> applyAttack(state, attack);
            case DealCommand.Defend defend -> applyDefend(state, defend);
            case DealCommand.Transfer transfer -> applyTransfer(state, transfer);
            case DealCommand.Pass pass -> applyPass(state, pass);
            case DealCommand.Take take -> applyTake(state, take);
            case DealCommand.HangCard hang -> applyHangCard(state, hang);
            case DealCommand.HangSkip skip -> applyHangSkip(state, skip);
            case DealCommand.ChooseTrump choose -> applyChooseTrump(state, choose);
            case DealCommand.RevealFaceDown reveal -> applyRevealFaceDown(state, reveal);
            case DealCommand.RevealFaceDownToDefend reveal -> applyRevealToDefend(state, reveal);
        };
    }

    /**
     * Козырь разыгран костью: победитель называет масть — любую из четырёх, в том числе
     * ту, которой у него на руках нет (§1.2). Сам джокер остаётся лежать нижней картой.
     */
    private MoveResult applyChooseTrump(final DealState state, final DealCommand.ChooseTrump command) {
        if (state.phase() != DealPhase.DICE) {
            return MoveResult.rejected(RejectionReason.TRUMP_NOT_IN_DISPUTE);
        }
        if (state.hiddenTrumpAwaitingSuit().isPresent()) {
            return chooseTrumpForHiddenTrump(state, command);
        }
        if (state.attackRightSeat() != command.seatNo()) {
            return MoveResult.rejected(RejectionReason.NOT_YOUR_TURN);
        }
        final Trump trump = Trump.of(command.suit());
        final int starter = lowestTrumpSeat(state, trump);
        return MoveResult.applied(state.toBuilder()
                .trump(trump)
                .phase(DealPhase.ATTACK)
                .roundStarterSeat(starter)
                .attackRightSeat(starter)
                .defenderSeat(state.nextActiveSeatAfter(starter))
                .build(), List.of(new DealEvent.TrumpChosen(command.seatNo(), command.suit())));
    }

    /**
     * ⭐ Потайной козырь оказался джокером: сначала кость и выбор масти, и только потом
     * карта уходит в руку добирающему (§1.9, ADR-035). Порядок раунда при этом уже
     * посчитан — первый ход по младшему козырю определяется только при сдаче.
     */
    private MoveResult chooseTrumpForHiddenTrump(final DealState state, final DealCommand.ChooseTrump command) {
        final PendingHiddenTrump pending = state.hiddenTrumpAwaitingSuit().orElseThrow();
        if (pending.chooserSeat() != command.seatNo()) {
            return MoveResult.rejected(RejectionReason.NOT_YOUR_TURN);
        }
        final PlayerState recipient = state.playerAt(pending.recipientSeat());
        final List<Card> hand = new ArrayList<>(recipient.hand());
        hand.add(pending.card());
        return MoveResult.applied(state.toBuilder()
                .trump(Trump.of(command.suit()))
                .player(recipient.withHand(List.copyOf(hand)))
                .pendingHiddenTrump(null)
                .phase(DealPhase.ATTACK)
                .build(), List.of(
                        new DealEvent.TrumpChosen(command.seatNo(), command.suit()),
                        new DealEvent.CardsDrawn(pending.recipientSeat(), List.of(pending.card()))));
    }

    /** Первый ход — у обладателя младшего козыря (§1.2), уже по выбранной масти. */
    private int lowestTrumpSeat(final DealState state, final Trump trump) {
        Rank lowest = null;
        int starter = 0;
        for (final PlayerState player : state.players()) {
            for (final Card card : player.hand()) {
                if (card instanceof PipCard pip && pip.suit() == trump.suit()
                        && (lowest == null || lowest.isHigherThan(pip.rank()))) {
                    lowest = pip.rank();
                    starter = player.seatNo();
                }
            }
        }
        return starter;
    }

    /**
     * ⭐ Вскрытие скрытой карты (§1.8). Команда не называет карту — игрок её не видит,
     * и назвать не может (ADR-026).
     *
     * <p>Вскрытие <b>необратимо и не зависит от исхода хода</b>: если открытая карта не
     * вписалась в атаку по рангу, ход ею не проходит, но карта уже в руке. Поэтому команда
     * не отклоняется — она применяется, просто с разным результатом.
     */
    private MoveResult applyRevealFaceDown(final DealState state, final DealCommand.RevealFaceDown command) {
        final MoveVerdict allowed = moveRules.canRevealFaceDown(state, command.seatNo());
        if (allowed instanceof MoveVerdict.Rejected rejected) {
            return MoveResult.rejected(rejected.reason());
        }
        final Card card = state.playerAt(command.seatNo()).faceDownCard();
        final DealState revealed = revealInHand(state, command.seatNo(), card);
        final List<DealEvent> events = new ArrayList<>();
        events.add(new DealEvent.FaceDownRevealed(command.seatNo(), card));

        final MoveResult attack = applyAttack(revealed, new DealCommand.Attack(command.seatNo(), card));
        if (attack instanceof MoveResult.Applied applied) {
            events.addAll(applied.events());
            return MoveResult.applied(applied.state(), events);
        }
        return MoveResult.applied(revealed, events);
    }

    /** То же для защиты: карта вскрывается, а дальше либо бьёт цель, либо остаётся в руке. */
    private MoveResult applyRevealToDefend(final DealState state,
                                           final DealCommand.RevealFaceDownToDefend command) {
        final MoveVerdict allowed = moveRules.canRevealFaceDown(state, command.seatNo());
        if (allowed instanceof MoveVerdict.Rejected rejected) {
            return MoveResult.rejected(rejected.reason());
        }
        final Card card = state.playerAt(command.seatNo()).faceDownCard();
        final DealState revealed = revealInHand(state, command.seatNo(), card);
        final List<DealEvent> events = new ArrayList<>();
        events.add(new DealEvent.FaceDownRevealed(command.seatNo(), card));

        final MoveResult defence = applyDefend(revealed,
                new DealCommand.Defend(command.seatNo(), card, command.target()));
        if (defence instanceof MoveResult.Applied applied) {
            events.addAll(applied.events());
            return MoveResult.applied(applied.state(), events);
        }
        return MoveResult.applied(revealed, events);
    }

    /** Скрытая карта переезжает в руку. Обратного пути нет ни в одном сценарии (§1.8). */
    private DealState revealInHand(final DealState state, final int seatNo, final Card card) {
        final PlayerState player = state.playerAt(seatNo);
        final List<Card> hand = new ArrayList<>(player.hand());
        hand.add(card);
        return state.toBuilder()
                .player(player.withFaceDownRevealed().withHand(List.copyOf(hand)))
                .build();
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
                .phase(state.phase() == DealPhase.TAKING ? DealPhase.TAKING : DealPhase.DEFEND)
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
        if (unbeatenRemain(table)) {
            return MoveResult.applied(defended, events);
        }
        return afterLastCardBeaten(defended, events);
    }

    /**
     * ⚠️ Отбита последняя карта на столе — раунд возвращается в атаку, и право подкидывать
     * обязано достаться тому, кто им реально может воспользоваться.
     *
     * <p>Пас при неотбитом столе право никуда не двигает: двигать его некуда, следующего
     * подкидывающего нет. Оно так и остаётся за спасовавшим, и после отбоя последней карты
     * этот игрок не может ни подкинуть ({@code hasPassed}), ни спасовать ещё раз — раздача
     * вставала намертво у всех за столом. Если подкидывать больше некому, раунд здесь же
     * и закрывается: стол отбит целиком, это «бито».
     */
    private MoveResult afterLastCardBeaten(final DealState state, final List<DealEvent> events) {
        final OptionalInt next = attackOrder.nextAttacker(state);
        if (next.isPresent()) {
            if (next.getAsInt() != state.attackRightSeat()) {
                events.add(new DealEvent.AttackRightMoved(next.getAsInt()));
            }
            return MoveResult.applied(state.toBuilder().attackRightSeat(next.getAsInt()).build(), events);
        }
        events.add(new DealEvent.RoundBeaten(state.defenderSeat(), state.tableCards()));
        return MoveResult.applied(finishRound(clearTable(state, true), false, events), events);
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
        if (state.table().isEmpty() && state.playerAt(command.seatNo()).canPlayFaceDown(state.isDeckEmpty())) {
            return MoveResult.rejected(RejectionReason.MUST_REVEAL_FACE_DOWN);
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
                    .phase(nextPhaseAfterPass(afterPass))
                    .build(), events);
        }
        if (afterPass.phase() == DealPhase.TAKING) {
            return MoveResult.applied(collectTable(afterPass, events), events);
        }
        if (unbeatenRemain(afterPass.table())) {
            return MoveResult.applied(afterPass.toBuilder().phase(DealPhase.DEFEND).build(), events);
        }
        events.add(new DealEvent.RoundBeaten(afterPass.defenderSeat(), afterPass.tableCards()));
        return MoveResult.applied(finishRound(clearTable(afterPass, true), false, events), events);
    }

    /**
     * «Беру» раунд не закрывает (ADR-038): подкидывающие докидывают карты, пока не спасуют,
     * и только тогда стол уезжает в руку. Потолком остаётся лимит раунда — рука взявшего
     * больше ничего не ограничивает, он всё равно заберёт всё.
     */
    private MoveResult applyTake(final DealState state, final DealCommand.Take command) {
        if (state.defenderSeat() != command.seatNo() || state.phase() == DealPhase.TAKING) {
            return MoveResult.rejected(RejectionReason.NOT_YOUR_TURN);
        }
        if (state.table().isEmpty()) {
            return MoveResult.rejected(RejectionReason.NOTHING_TO_TAKE);
        }
        final List<DealEvent> events = new ArrayList<>();
        events.add(new DealEvent.TakeAnnounced(command.seatNo()));
        final DealState taking = state.toBuilder().phase(DealPhase.TAKING).build();
        if (attackOrder.nextAttacker(taking).isPresent()) {
            return MoveResult.applied(taking, events);
        }
        return MoveResult.applied(collectTable(taking, events), events);
    }

    /** Стол уезжает в руку взявшему, и раунд закрывается. */
    private DealState collectTable(final DealState state, final List<DealEvent> events) {
        final List<Card> taken = state.tableCards();
        final List<Card> hand = new ArrayList<>(state.defender().hand());
        hand.addAll(taken);
        final PlayerState defender = state.defender().withHand(List.copyOf(hand));
        events.add(new DealEvent.CardsTaken(state.defenderSeat(), taken));
        return openHangingWindow(clearTable(state.toBuilder().player(defender).build(), false), events);
    }

    /**
     * ⭐ Окно навеса открывается на взявшего — и только сейчас, когда состав его руки уже
     * окончателен (§2.3, ADR-038). Если нужной карты нет ни у кого, окно не открывается
     * вовсе: навес просто не происходит.
     */
    private DealState openHangingWindow(final DealState state, final List<DealEvent> events) {
        if (!config.navesEnabled()) {
            return finishRound(state, true, events);
        }
        final int victim = state.defenderSeat();
        final List<Integer> holders = hangingRules.seatsHoldingFlyingCard(state, victim);
        if (holders.isEmpty()) {
            return finishRound(state, true, events);
        }
        events.add(new DealEvent.HangingWindowOpened(victim));
        return state.toBuilder()
                .phase(DealPhase.HANGING)
                .hangingWindow(HangingWindow.open(victim, steps(state, victim, holders),
                        hangingRules.isEveryClaimantHanging(state, victim)))
                .build();
    }

    /**
     * Ступени права. При джокере и при уникальном отстающем ступень одна — право сразу
     * у всех. В обычном случае их три: атаковавший, поддержавший и все остальные наравне
     * (§2.3).
     */
    private List<List<Integer>> steps(final DealState state, final int victim, final List<Integer> holders) {
        if (hangingRules.isRightEqualForAll(state, victim)) {
            return List.of(holders);
        }
        final List<Integer> priority = hangingRules.priorityOrder(state, victim);
        final List<List<Integer>> steps = new ArrayList<>();
        for (int tier = 0; tier < 2 && tier < priority.size(); tier++) {
            final int seat = priority.get(tier);
            if (holders.contains(seat)) {
                steps.add(List.of(seat));
            }
        }
        final List<Integer> rest = holders.stream()
                .filter(seat -> priority.indexOf(seat) >= 2)
                .toList();
        if (!rest.isEmpty()) {
            steps.add(rest);
        }
        return List.copyOf(steps);
    }

    private MoveResult applyHangCard(final DealState state, final DealCommand.HangCard command) {
        final HangingWindow window = state.hangingWindow();
        if (state.phase() != DealPhase.HANGING || window == null
                || !window.isSeatOnCurrentStep(command.seatNo())) {
            return MoveResult.rejected(RejectionReason.NOT_IN_HANGING_WINDOW);
        }
        final MoveVerdict verdict = hangingRules.canHang(state, command.seatNo(),
                window.victimSeat(), command.card());
        if (verdict instanceof MoveVerdict.Rejected rejected) {
            return MoveResult.rejected(rejected.reason());
        }
        final List<DealEvent> events = new ArrayList<>();
        final HangingWindow claimed = window.withClaim(new HangClaim(command.seatNo(), command.card()));
        return MoveResult.applied(advanceWindow(state.toBuilder().hangingWindow(claimed).build(), events), events);
    }

    private MoveResult applyHangSkip(final DealState state, final DealCommand.HangSkip command) {
        final HangingWindow window = state.hangingWindow();
        if (state.phase() != DealPhase.HANGING || window == null
                || !window.isSeatOnCurrentStep(command.seatNo())) {
            return MoveResult.rejected(RejectionReason.NOT_IN_HANGING_WINDOW);
        }
        final List<DealEvent> events = new ArrayList<>();
        final HangingWindow declined = window.withDecline(command.seatNo());
        return MoveResult.applied(advanceWindow(state.toBuilder().hangingWindow(declined).build(), events), events);
    }

    /**
     * Ступень исчерпана — разрешаем её. Заявок нет: право уходит дальше по очереди, а если
     * очередь кончилась — окно закрывается без навеса.
     */
    private DealState advanceWindow(final DealState state, final List<DealEvent> events) {
        final HangingWindow window = state.hangingWindow();
        if (!window.isStepComplete()) {
            return state;
        }
        if (window.claims().isEmpty()) {
            if (window.hasNextStep()) {
                return state.toBuilder().hangingWindow(window.nextStep()).build();
            }
            return closeWindow(state, events);
        }
        return closeWindow(applyClaims(state, window, events), events);
    }

    /**
     * ⭐ Уровень поднимается ровно на одну ступень за окно, сколько бы карт в слот ни ушло
     * (§2.3). При правиле отстающего навешивают все заявившиеся; в остальных случаях —
     * один, и спор решается костью, а не тем, кто успел (ADR-029).
     */
    private DealState applyClaims(final DealState state, final HangingWindow window,
                                  final List<DealEvent> events) {
        final List<HangClaim> winners = selectWinners(state, window, events);
        DealState current = state;
        for (final HangClaim claim : winners) {
            current = current.toBuilder()
                    .player(current.playerAt(claim.seatNo()).withoutCard(claim.card()))
                    .build();
            current = current.toBuilder()
                    .player(current.playerAt(window.victimSeat()).withHungCard(claim.card(), claim.seatNo()))
                    .build();
            events.add(new DealEvent.CardHung(claim.seatNo(), window.victimSeat(), claim.card()));
        }
        final int level = current.playerAt(window.victimSeat()).navesLevel() + 1;
        events.add(new DealEvent.NavesLevelChanged(window.victimSeat(), level));
        return current.toBuilder()
                .player(current.playerAt(window.victimSeat()).withNavesLevel(level))
                .build();
    }

    private List<HangClaim> selectWinners(final DealState state, final HangingWindow window,
                                          final List<DealEvent> events) {
        if (window.everyClaimantHangs()) {
            return window.claims();
        }
        if (window.claims().size() == 1) {
            return window.claims();
        }
        final List<Integer> participants = window.claims().stream().map(HangClaim::seatNo).toList();
        final int winner = dice.winnerAmong(participants, state.rngSeed(), state.diceRolls());
        events.add(new DealEvent.DiceRolled(winner, participants));
        return window.claims().stream().filter(claim -> claim.seatNo() == winner).toList();
    }

    private DealState closeWindow(final DealState state, final List<DealEvent> events) {
        final HangingWindow window = state.hangingWindow();
        events.add(new DealEvent.HangingWindowClosed(window.victimSeat()));
        final int rolls = state.diceRolls() + (window.claims().size() > 1 ? 1 : 0);
        return finishRound(state.toBuilder()
                .hangingWindow(null)
                .diceRolls(rolls)
                .build(), true, events);
    }

    /**
     * Закрытие раунда: стол в отбой или в руку, добор в строгом порядке, выход игроков,
     * проверка конца раздачи. Порядок шагов не переставляется — добор обязан случиться
     * до проверки выхода, иначе игрок вышел бы, не получив причитающихся карт.
     *
     * <p>🟨 Навесы (§2.3) вклиниваются сюда после «взял» и появятся в M5.
     */
    private DealState finishRound(final DealState state, final boolean taken, final List<DealEvent> events) {
        final DealState refilled = refill(state, events);
        final DealState afterExits = markExits(refilled, events);
        if (afterExits.playersInDeal() <= 1) {
            final int loser = lastPlayerInDeal(afterExits);
            events.add(new DealEvent.DealFinished(loser));
            return afterExits.toBuilder().phase(DealPhase.DEAL_OVER).build();
        }
        final DealState nextRound = startNextRound(afterExits, taken);
        return nextRound.hiddenTrumpAwaitingSuit()
                .map(pending -> nextRound.toBuilder().phase(DealPhase.DICE).build())
                .orElse(nextRound);
    }

    /**
     * ⭐ Стол уезжает со стола ровно в тот момент, когда раунд закрылся, — в отбой или
     * в руку. Раньше это делалось в конце закрытия, и между «взял» и очисткой успевало
     * вклиниться окно навеса: карты одновременно лежали и в руке взявшего, и на столе.
     *
     * <p>Заодно запоминается состав последней атаки: он нужен для степеней проигрыша
     * и после очистки его уже не восстановить (§0.3).
     */
    private DealState clearTable(final DealState state, final boolean discarded) {
        return state.toBuilder()
                .lastAttackCards(state.table().stream().map(TableSlot::attack).toList())
                .table(List.of())
                .anyPileDiscarded(state.anyPileDiscarded() || discarded)
                .build();
    }

    /**
     * Добор до {@code dealSize} в порядке §1.4.1: начавший раунд → второй сосед → остальные
     * по часовой → защищавшийся последним. Порядок не формальность: колода конечна, и тот,
     * кто добирает раньше, успевает взять карты, которых не хватит остальным.
     */
    private DealState refill(final DealState state, final List<DealEvent> events) {
        final List<Card> deck = new ArrayList<>(state.deck());
        DealState current = state;
        PendingHiddenTrump pending = null;
        Trump newTrump = null;
        for (final int seat : refillOrder(state)) {
            final PlayerState player = current.playerAt(seat);
            final List<Card> drawn = new ArrayList<>();
            while (player.handSize() + drawn.size() < config.dealSize() && !deck.isEmpty()) {
                final boolean lastCard = deck.size() == 1;
                final Card card = deck.remove(0);
                if (!lastCard) {
                    drawn.add(card);
                    continue;
                }
                events.add(new DealEvent.HiddenTrumpRevealed(seat, card));
                if (card instanceof PipCard pip) {
                    newTrump = Trump.of(pip.suit());
                    events.add(new DealEvent.TrumpChanged(seat, pip.suit()));
                    drawn.add(card);
                } else {
                    pending = new PendingHiddenTrump(card, seat,
                            dice.winnerAmong(seatsInDeal(current), current.rngSeed(), current.diceRolls()));
                }
            }
            if (drawn.isEmpty()) {
                continue;
            }
            final List<Card> hand = new ArrayList<>(player.hand());
            hand.addAll(drawn);
            events.add(new DealEvent.CardsDrawn(seat, List.copyOf(drawn)));
            current = current.toBuilder().player(player.withHand(List.copyOf(hand))).build();
        }
        final DealState.Builder builder = current.toBuilder().deck(List.copyOf(deck));
        if (newTrump != null) {
            builder.trump(newTrump);
        }
        if (pending != null) {
            builder.pendingHiddenTrump(pending).diceRolls(current.diceRolls() + 1);
        }
        return builder.build();
    }

    private List<Integer> seatsInDeal(final DealState state) {
        return state.players().stream()
                .filter(PlayerState::inDeal)
                .map(PlayerState::seatNo)
                .toList();
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
            current = current.toBuilder().player(player.leftDeal()).build();
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
            return player.withoutCard(card);
        }
        events.add(new DealEvent.FaceDownRevealed(seatNo, card));
        return player.withFaceDownRevealed();
    }

    /**
     * Фаза после паса, когда право ушло дальше. Объявленное «беру» переживает пас: пока
     * подкидывающие не закончатся, раунд остаётся в {@link DealPhase#TAKING}.
     */
    private DealPhase nextPhaseAfterPass(final DealState state) {
        if (state.phase() == DealPhase.TAKING) {
            return DealPhase.TAKING;
        }
        return unbeatenRemain(state.table()) ? DealPhase.DEFEND : DealPhase.ATTACK;
    }

    private boolean unbeatenRemain(final List<TableSlot> table) {
        return table.stream().anyMatch(slot -> !slot.isBeaten());
    }
}
