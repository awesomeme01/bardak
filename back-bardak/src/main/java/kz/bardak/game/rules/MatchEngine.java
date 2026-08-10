package kz.bardak.game.rules;

import java.util.ArrayList;
import java.util.List;
import java.util.Objects;

/**
 * Автомат матча (§4.1): раздача → итог → перераздача, пока кому-то не навесят джокер.
 *
 * <p>⭐ Раздача заканчивается — матч не обязан. «Карты кончились» это штатное завершение
 * раздачи, а не вырождение партии (§0.6): уровни переносятся, колода собирается заново,
 * и игра продолжается.
 *
 * <p>Матч закончен только тогда, когда после всех сдвигов у кого-то остался джокер. Джокер
 * сам по себе конца не означает: выход первым его снимает (corner case 1).
 */
public final class MatchEngine {

    private final RulesConfig config;
    private final DealEngine dealEngine;
    private final DealScoring scoring;
    private final Dealer dealer;

    public MatchEngine(final RulesConfig config, final AttackOrderPolicy attackOrder, final DiceResolver dice) {
        this.config = Objects.requireNonNull(config, "config");
        this.dealEngine = new DealEngine(config, attackOrder, dice);
        this.scoring = new DealScoring(config);
        this.dealer = new Dealer(config, dice);
    }

    public static MatchEngine withDefaults() {
        return new MatchEngine(RulesConfig.defaults(),
                new AttackOrderPolicy.BardakStrictNeighbours(), new DiceResolver.Seeded());
    }

    /** Новый матч: все на «летит 6», первая раздача сдана. */
    public MatchState startMatch(final int playerCount, final long matchSeed) {
        final List<Integer> levels = new ArrayList<>();
        for (int seat = 0; seat < playerCount; seat++) {
            levels.add(NavesScale.NO_NAVES);
        }
        final DealState deal = dealer.startDeal(levels, dealSeed(matchSeed, 1));
        return new MatchState(MatchPhase.IN_DEAL, List.copyOf(levels), 1, matchSeed, deal, List.of());
    }

    /**
     * Команда игрока. Пока раздача не кончилась, матч просто передаёт её движку раздачи;
     * на {@link DealPhase#DEAL_OVER} считается итог и начинается следующая раздача — либо
     * матч заканчивается.
     */
    public MatchResult apply(final MatchState state, final DealCommand command) {
        Objects.requireNonNull(state, "state");
        Objects.requireNonNull(command, "command");
        if (state.isOver()) {
            return MatchResult.rejected(RejectionReason.NOT_YOUR_TURN);
        }
        final MoveResult moveResult = dealEngine.apply(state.deal(), command);
        if (moveResult instanceof MoveResult.Rejected rejected) {
            return MatchResult.rejected(rejected.reason());
        }
        final MoveResult.Applied applied = (MoveResult.Applied) moveResult;
        if (applied.state().phase() == DealPhase.DEAL_OVER) {
            return closeDeal(state, applied);
        }
        return MatchResult.applied(state.toBuilder()
                .deal(reshuffleIfNobodyHasTrump(state, applied.state()))
                .build(), applied.events());
    }

    /**
     * ⭐ Козырь могли назвать костью — и назвать масть, которой нет ни у кого (§1.2).
     * Тогда первый ход определять не из чего, и раздача пересдаётся, как и при козыре
     * с нижней карты (OQ-22).
     */
    private DealState reshuffleIfNobodyHasTrump(final MatchState state, final DealState deal) {
        if (!deal.hasTrump() || deal.phase() != DealPhase.ATTACK || !deal.table().isEmpty()
                || dealer.hasAnyTrumpInHands(deal)) {
            return deal;
        }
        return dealer.startDeal(state.navesLevels(),
                dealer.reshuffleSeed(dealSeed(state.matchSeed(), state.dealNo()), 0));
    }

    /**
     * Раздача сыграна: считаем итог, переносим уровни и либо заканчиваем матч, либо
     * сдаём заново.
     *
     * <p>⭐ Карты из слотов, включая джокеры, возвращаются в игру сами собой: колода
     * собирается заново по составу стола (§2.3). Переносится только уровень.
     */
    private MatchResult closeDeal(final MatchState state, final MoveResult.Applied applied) {
        final DealOutcome outcome = scoring.score(applied.state());
        final List<Integer> levels = outcome.players().stream().map(PlayerOutcome::levelAfter).toList();
        final List<DealOutcome> results = new ArrayList<>(state.results());
        results.add(outcome);

        final MatchState.Builder next = state.toBuilder()
                .navesLevels(levels)
                .results(List.copyOf(results))
                .deal(applied.state());
        if (outcome.isMatchOver()) {
            return MatchResult.applied(next.phase(MatchPhase.MATCH_OVER).build(), applied.events());
        }
        final int dealNo = state.dealNo() + 1;
        return MatchResult.applied(next
                .dealNo(dealNo)
                .deal(dealer.startDeal(levels, dealSeed(state.matchSeed(), dealNo)))
                .build(), applied.events());
    }

    /**
     * Под-seed раздачи. Весь матч воспроизводим по паре «seed матча + последовательность
     * команд», включая перераздачи (§6).
     */
    private long dealSeed(final long matchSeed, final int dealNo) {
        return matchSeed * 1_000_003L + dealNo;
    }
}
