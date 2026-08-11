package kz.bardak.game.protocol;

import static org.assertj.core.api.Assertions.assertThat;

import com.fasterxml.jackson.databind.ObjectMapper;
import java.util.List;
import kz.bardak.game.rules.DealCommand;
import kz.bardak.game.rules.MatchEngine;
import kz.bardak.game.rules.MatchResult;
import kz.bardak.game.rules.MatchState;
import kz.bardak.game.rules.PlayerState;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.ValueSource;

/**
 * Снимок состояния матча.
 *
 * <p>⭐ Кодек написан руками, чтобы не тащить JSON в чистый движок, — и цена этого решения
 * оплачивается ровно здесь: снимок, разобранный обратно, обязан совпасть с исходным
 * состоянием целиком, а не «по важным полям».
 */
class MatchStateCodecTest {

    private final MatchStateCodec codec = new MatchStateCodec(new ObjectMapper());
    private final MatchEngine engine = MatchEngine.withDefaults();

    @DisplayName("Should restore the state exactly When a fresh match is round-tripped")
    @ParameterizedTest
    @ValueSource(ints = {2, 3, 4, 5})
    void shouldRestoreTheStateExactlyWhenAFreshMatchIsRoundTripped(final int playerCount) {
        final MatchState state = engine.startMatch(playerCount, 20260811L);

        assertThat(codec.decode(codec.encode(state))).isEqualTo(state);
    }

    @DisplayName("Should restore the state exactly When the match is in the middle of a round")
    @Test
    void shouldRestoreTheStateExactlyWhenTheMatchIsInTheMiddleOfARound() {
        final MatchState played = playAWhile(engine.startMatch(3, 77L), 25);

        final MatchState restored = codec.decode(codec.encode(played));

        assertThat(restored).isEqualTo(played);
        assertThat(restored.deal().players()).isEqualTo(played.deal().players());
        assertThat(restored.deal().table()).isEqualTo(played.deal().table());
    }

    @DisplayName("Should keep the mid-round flags When a card has already been beaten")
    @Test
    void shouldKeepTheMidRoundFlagsWhenACardHasAlreadyBeenBeaten() {
        // ⭐ Ищем состояние с поднятым флагом отбоя намеренно: он выключает перевод (ADR-031),
        // и снимок, потерявший его, вернул бы игроку право, которого у него уже нет.
        MatchState state = engine.startMatch(3, 13L);
        for (int move = 0; move < 500 && !state.deal().anyCardBeatenThisRound(); move++) {
            state = step(state);
        }
        assertThat(state.deal().anyCardBeatenThisRound())
                .withFailMessage("Не нашлось состояния с отбитой картой — тест бесполезен")
                .isTrue();

        final MatchState restored = codec.decode(codec.encode(state));

        assertThat(restored.deal().anyCardBeatenThisRound()).isTrue();
        assertThat(restored).isEqualTo(state);
    }

    @DisplayName("Should keep the hidden card of every player When the state is round-tripped")
    @Test
    void shouldKeepTheHiddenCardOfEveryPlayerWhenTheStateIsRoundTripped() {
        final MatchState state = engine.startMatch(4, 5L);

        final MatchState restored = codec.decode(codec.encode(state));

        // Снимок внутренний: скрытые карты в нём есть и обязаны пережить круг —
        // иначе восстановленный матч играется другой колодой.
        assertThat(restored.deal().players()).extracting(PlayerState::faceDownCard)
                .isEqualTo(state.deal().players().stream().map(PlayerState::faceDownCard).toList());
    }

    @DisplayName("Should survive the deal boundary When several deals are already played")
    @Test
    void shouldSurviveTheDealBoundaryWhenSeveralDealsAreAlreadyPlayed() {
        MatchState state = engine.startMatch(2, 31L);
        for (int move = 0; move < 400 && state.dealNo() < 2; move++) {
            state = step(state);
        }

        final MatchState restored = codec.decode(codec.encode(state));

        assertThat(restored.dealNo()).isEqualTo(state.dealNo());
        assertThat(restored.results()).isEqualTo(state.results());
        assertThat(restored.navesLevels()).isEqualTo(state.navesLevels());
        assertThat(restored).isEqualTo(state);
    }

    @DisplayName("Should continue identically When play resumes from the restored state")
    @Test
    void shouldContinueIdenticallyWhenPlayResumesFromTheRestoredState() {
        final MatchState original = playAWhile(engine.startMatch(3, 99L), 12);
        final MatchState restored = codec.decode(codec.encode(original));

        // ⭐ Главное свойство снимка: из него игра продолжается так же, как из оригинала.
        assertThat(playAWhile(restored, 10)).isEqualTo(playAWhile(original, 10));
    }

    private MatchState playAWhile(final MatchState start, final int moves) {
        MatchState state = start;
        for (int move = 0; move < moves && !state.isOver(); move++) {
            state = step(state);
        }
        return state;
    }

    /** Один ход простейшего автоигрока: первое, что примет движок. */
    private MatchState step(final MatchState state) {
        for (final DealCommand candidate : candidates(state)) {
            final MatchResult result = engine.apply(state, candidate);
            if (result instanceof MatchResult.Applied applied) {
                return applied.state();
            }
        }
        return state;
    }

    private List<DealCommand> candidates(final MatchState state) {
        final var deal = state.deal();
        final var commands = new java.util.ArrayList<DealCommand>();
        for (final var suit : kz.bardak.game.rules.Suit.values()) {
            commands.add(new DealCommand.ChooseTrump(deal.attackRightSeat(), suit));
        }
        for (final var card : deal.playerAt(deal.defenderSeat()).hand()) {
            for (final var slot : deal.table()) {
                commands.add(new DealCommand.Defend(deal.defenderSeat(), card, slot.attack()));
            }
        }
        for (final var card : deal.playerAt(deal.attackRightSeat()).hand()) {
            commands.add(new DealCommand.Attack(deal.attackRightSeat(), card));
        }
        for (final var seat : deal.hanging().map(w -> w.currentStep()).orElse(List.of())) {
            for (final var card : deal.playerAt(seat).hand()) {
                commands.add(new DealCommand.HangCard(seat, card));
            }
            commands.add(new DealCommand.HangSkip(seat));
        }
        commands.add(new DealCommand.Pass(deal.attackRightSeat()));
        commands.add(new DealCommand.Take(deal.defenderSeat()));
        commands.add(new DealCommand.RevealFaceDown(deal.attackRightSeat()));
        return commands;
    }
}
