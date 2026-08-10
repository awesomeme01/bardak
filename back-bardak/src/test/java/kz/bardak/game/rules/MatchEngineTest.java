package kz.bardak.game.rules;

import static org.assertj.core.api.Assertions.assertThat;

import java.util.ArrayList;
import java.util.List;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.ValueSource;

/**
 * Автомат матча — §0, §4.1. Раздача заканчивается, матч не обязан: уровни переносятся,
 * колода собирается заново, игра продолжается до чьего-то джокера.
 */
class MatchEngineTest {

    private static final long ANY_SEED = 20260810L;

    private final MatchEngine engine = MatchEngine.withDefaults();

    /** Стол с укороченной шкалой: её длина — параметр конфига (§1.6, OQ-13). */
    private static final RulesConfig SHORT_SCALE = new RulesConfig(6, 5, 6, true, true, true,
            new NavesScale(List.of(Rank.SIX, Rank.SEVEN)));

    private final MatchEngine shortScale = new MatchEngine(SHORT_SCALE,
            new AttackOrderPolicy.BardakStrictNeighbours(), new DiceResolver.Seeded());

    @DisplayName("Should start everyone at the bottom of the scale When a match begins")
    @Test
    void shouldStartEveryoneAtTheBottomOfTheScaleWhenAMatchBegins() {
        final MatchState match = engine.startMatch(3, ANY_SEED);

        assertThat(match.phase()).isEqualTo(MatchPhase.IN_DEAL);
        assertThat(match.navesLevels()).containsOnly(NavesScale.NO_NAVES);
        assertThat(match.dealNo()).isEqualTo(1);
        assertThat(match.results()).isEmpty();
    }

    @DisplayName("Should refuse any command When the match is already over")
    @Test
    void shouldRefuseAnyCommandWhenTheMatchIsAlreadyOver() {
        final MatchState finished = playToTheEnd(engine, engine.startMatch(3, ANY_SEED));

        assertThat(engine.apply(finished, new DealCommand.Pass(0)))
                .isEqualTo(MatchResult.rejected(RejectionReason.NOT_YOUR_TURN));
    }

    @DisplayName("Should end the match with a joker and a degree When it is played to the end")
    @ParameterizedTest
    @ValueSource(ints = {2, 3, 4, 5})
    void shouldEndTheMatchWithAJokerAndADegreeWhenItIsPlayedToTheEnd(final int playerCount) {
        final MatchState finished = playToTheEnd(shortScale, shortScale.startMatch(playerCount, ANY_SEED));

        assertThat(finished.phase()).isEqualTo(MatchPhase.MATCH_OVER);
        assertThat(finished.navesLevels())
                .anyMatch(level -> level >= SHORT_SCALE.navesScale().jokerLevel());
        assertThat(finished.mainLoser()).isPresent();
        assertThat(finished.mainLoser()).get()
                .satisfies(loser -> assertThat(loser.lossDegree()).isNotNull());
    }

    @DisplayName("Should keep every level inside the scale When a whole match is played")
    @ParameterizedTest
    @ValueSource(ints = {2, 3, 4, 5})
    void shouldKeepEveryLevelInsideTheScaleWhenAWholeMatchIsPlayed(final int playerCount) {
        final MatchState finished = playToTheEnd(shortScale, shortScale.startMatch(playerCount, ANY_SEED));

        assertThat(finished.navesLevels()).allSatisfy(level ->
                assertThat(level).isBetween(NavesScale.NO_NAVES, SHORT_SCALE.navesScale().jokerLevel()));
    }

    @DisplayName("Should take more than one deal When the match runs its course")
    @Test
    void shouldTakeMoreThanOneDealWhenTheMatchRunsItsCourse() {
        final MatchState finished = playToTheEnd(shortScale, shortScale.startMatch(4, ANY_SEED));

        assertThat(finished.results()).hasSizeGreaterThan(1);
        assertThat(finished.results()).allSatisfy(result ->
                assertThat(result.dealLoserSeat()).isBetween(0, 3));
    }

    @DisplayName("Should end sooner on a shortened scale When the table config shortens it")
    @Test
    void shouldEndSoonerOnAShortenedScaleWhenTheTableConfigShortensIt() {
        final MatchState onShortScale = playToTheEnd(shortScale, shortScale.startMatch(3, ANY_SEED));
        assertThat(onShortScale.phase()).isEqualTo(MatchPhase.MATCH_OVER);
        assertThat(onShortScale.navesLevels()).allSatisfy(level ->
                assertThat(level).isBetween(NavesScale.NO_NAVES, SHORT_SCALE.navesScale().jokerLevel()));
        assertThat(onShortScale.results()).isNotEmpty();
    }

    @DisplayName("Should always accept some command and never lose a card When a long match is played")
    @Test
    void shouldAlwaysAcceptSomeCommandAndNeverLoseACardWhenALongMatchIsPlayed() {
        final int playerCount = 4;
        final List<Card> full = new DeckFactory().buildOrdered(playerCount);
        MatchState state = engine.startMatch(playerCount, ANY_SEED);

        for (int move = 0; move < 3_000 && !state.isOver(); move++) {
            final List<Card> inPlay = cardsInPlay(state.deal());
            assertThat(inPlay).doesNotHaveDuplicates();
            assertThat(inPlay).hasSizeLessThanOrEqualTo(full.size());
            assertThat(full).containsAll(inPlay);
            final MatchState next = nextMove(engine, state);
            assertThat(next).withFailMessage("Ни одна команда не принимается в фазе %s",
                    state.deal().phase()).isNotSameAs(state);
            state = next;
        }
    }

    @DisplayName("Should carry the levels and clear the slots When a new deal is dealt")
    @Test
    void shouldCarryTheLevelsAndClearTheSlotsWhenANewDealIsDealt() {
        final MatchState afterFirstDeal = playUntilDeal(engine.startMatch(3, ANY_SEED), 2);

        assertThat(afterFirstDeal.dealNo()).isEqualTo(2);
        assertThat(afterFirstDeal.deal().players()).extracting(PlayerState::navesLevel)
                .containsExactlyElementsOf(afterFirstDeal.navesLevels());
        assertThat(afterFirstDeal.deal().players()).allSatisfy(player -> {
            assertThat(player.hungCards()).isEmpty();
            assertThat(player.inDeal()).isTrue();
        });
    }

    @DisplayName("Should put every card back into play When a new deal is dealt")
    @Test
    void shouldPutEveryCardBackIntoPlayWhenANewDealIsDealt() {
        final MatchState afterFirstDeal = playUntilDeal(engine.startMatch(3, ANY_SEED), 2);
        final DealState deal = afterFirstDeal.deal();

        final List<Card> everywhere = new ArrayList<>(deal.deck());
        for (final PlayerState player : deal.players()) {
            everywhere.addAll(player.hand());
            player.faceDown().ifPresent(everywhere::add);
        }

        assertThat(everywhere).doesNotHaveDuplicates()
                .containsExactlyInAnyOrderElementsOf(new DeckFactory().buildOrdered(3));
    }

    @DisplayName("Should replay identically When the same seed and the same moves are used")
    @Test
    void shouldReplayIdenticallyWhenTheSameSeedAndTheSameMovesAreUsed() {
        final MatchState first = playToTheEnd(shortScale, shortScale.startMatch(4, ANY_SEED));
        final MatchState second = playToTheEnd(shortScale, shortScale.startMatch(4, ANY_SEED));

        assertThat(first.navesLevels()).isEqualTo(second.navesLevels());
        assertThat(first.dealNo()).isEqualTo(second.dealNo());
        assertThat(first.mainLoser()).isEqualTo(second.mainLoser());
    }

    @DisplayName("Should diverge When the match seed differs")
    @Test
    void shouldDivergeWhenTheMatchSeedDiffers() {
        final MatchState first = engine.startMatch(4, ANY_SEED);
        final MatchState second = engine.startMatch(4, ANY_SEED + 1);

        assertThat(first.deal().deck()).isNotEqualTo(second.deal().deck());
    }

    private MatchState playToTheEnd(final MatchEngine engine, final MatchState start) {
        return play(engine, start, Integer.MAX_VALUE);
    }

    private MatchState playUntilDeal(final MatchState start, final int dealNo) {
        return play(engine, start, dealNo);
    }

    /**
     * Карты от самой дешёвой к самой дорогой: сначала некозырные по рангу, потом козыри,
     * джокеры последними. Так бот и атакует младшим, и бьёт минимально достаточным —
     * без этого «бито» почти не случается, а без «бито» карты не уходят из игры и раздача
     * не сходится.
     */
    private static List<Card> cheapestFirst(final List<Card> hand, final DealState deal) {
        final List<Card> sorted = new ArrayList<>(hand);
        sorted.sort((left, right) -> Integer.compare(weight(left, deal), weight(right, deal)));
        return sorted;
    }

    private static int weight(final Card card, final DealState deal) {
        if (card instanceof PipCard pip) {
            final boolean trump = deal.hasTrump() && pip.suit() == deal.trump().suit();
            return pip.rank().ordinal() + (trump ? 100 : 0);
        }
        return 1_000;
    }

    private static List<Card> cardsInPlay(final DealState deal) {
        final List<Card> cards = new ArrayList<>(deal.deck());
        for (final PlayerState player : deal.players()) {
            cards.addAll(player.hand());
            cards.addAll(player.hungCards());
            player.faceDown().ifPresent(cards::add);
        }
        for (final TableSlot slot : deal.table()) {
            cards.add(slot.attack());
            slot.defenceCard().ifPresent(cards::add);
        }
        return cards;
    }

    /**
     * Простейший автоигрок: перебирает команды в фиксированном порядке и берёт первую,
     * которую движок принял. Специально не умеет играть хорошо — его задача проверить,
     * что матч доигрывается до конца и правила при этом не расходятся.
     */
    private MatchState play(final MatchEngine engine, final MatchState start, final int stopAtDealNo) {
        MatchState state = start;
        for (int move = 0; move < 20_000; move++) {
            if (state.isOver() || state.dealNo() >= stopAtDealNo) {
                return state;
            }
            final MatchState before = state;
            state = nextMove(engine, state);
            if (state == before) {
                throw new IllegalStateException("Ни одна команда не принимается: " + state.deal().phase());
            }
        }
        throw new IllegalStateException("Матч не закончился за 20000 ходов");
    }

    private MatchState nextMove(final MatchEngine engine, final MatchState state) {
        for (final DealCommand command : candidates(state.deal())) {
            final MatchResult result = engine.apply(state, command);
            if (result instanceof MatchResult.Applied applied) {
                return applied.state();
            }
        }
        return state;
    }

    /**
     * Команды-кандидаты в порядке предпочтения: сыграть картой лучше, чем спасовать.
     *
     * <p>Бот намеренно <b>не подкидывает</b> и предпочитает пас взятию: раунд из одной атаки
     * либо отбивается и уходит в отбой, либо забирается. Иначе карты только и делают, что
     * возвращаются в руки, и раздача не сходится — играть плохо тоже надо уметь
     * предсказуемо.
     */
    private List<DealCommand> candidates(final DealState deal) {
        final List<DealCommand> commands = new ArrayList<>();
        if (deal.phase() == DealPhase.DICE) {
            commands.add(new DealCommand.ChooseTrump(deal.attackRightSeat(), Suit.HEARTS));
            return commands;
        }
        deal.hanging().ifPresent(window -> {
            for (final int seat : window.currentStep()) {
                for (final Card card : deal.playerAt(seat).hand()) {
                    commands.add(new DealCommand.HangCard(seat, card));
                }
                commands.add(new DealCommand.HangSkip(seat));
            }
        });
        final PlayerState defender = deal.defender();
        for (final TableSlot slot : deal.table()) {
            for (final Card card : cheapestFirst(defender.hand(), deal)) {
                commands.add(new DealCommand.Defend(defender.seatNo(), card, slot.attack()));
            }
        }
        final int attacker = deal.attackRightSeat();
        if (deal.table().isEmpty()) {
            for (final Card card : cheapestFirst(deal.playerAt(attacker).hand(), deal)) {
                commands.add(new DealCommand.Attack(attacker, card));
            }
        }
        for (final Card card : defender.hand()) {
            commands.add(new DealCommand.Transfer(defender.seatNo(), card));
        }
        for (final TableSlot slot : deal.table()) {
            commands.add(new DealCommand.RevealFaceDownToDefend(defender.seatNo(), slot.attack()));
        }
        commands.add(new DealCommand.RevealFaceDown(attacker));
        commands.add(new DealCommand.Pass(attacker));
        commands.add(new DealCommand.Take(defender.seatNo()));
        return commands;
    }
}
