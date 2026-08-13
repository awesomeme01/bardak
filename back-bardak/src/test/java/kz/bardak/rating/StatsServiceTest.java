package kz.bardak.rating;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.when;

import java.math.BigDecimal;
import java.util.List;
import java.util.UUID;
import kz.bardak.game.rules.LossDegree;
import kz.bardak.history.domain.MatchPlayerRecord;
import kz.bardak.history.domain.MatchPlayerRepository;
import kz.bardak.history.domain.MatchRecord;
import kz.bardak.history.domain.MatchRecordRepository;
import kz.bardak.rating.api.StatsDtos;
import kz.bardak.rating.domain.RatingHistoryEntry;
import kz.bardak.rating.domain.RatingHistoryRepository;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

/**
 * Статистика игрока без базы.
 *
 * <p>⭐ Проверяется то, что легко посчитать правдоподобно, но неверно: серия считается
 * по местам, а не по знаку рейтинга — в матче на пятерых можно занять второе место
 * и всё равно потерять очки.
 */
@ExtendWith(MockitoExtension.class)
class StatsServiceTest {

    private static final UUID PLAYER = UUID.randomUUID();

    @Mock
    private MatchPlayerRepository matchPlayers;

    @Mock
    private MatchRecordRepository matches;

    @Mock
    private RatingHistoryRepository histories;

    @DisplayName("Should count nothing When the player has not finished a match")
    @Test
    void shouldCountNothingWhenThePlayerHasNotFinishedAMatch() {
        when(matchPlayers.findByUserId(PLAYER)).thenReturn(List.of(unfinished()));

        final StatsDtos.PlayerStats stats = service().of(PLAYER);

        // Строка есть, места нет — матч идёт или отменён, и в статистику он не идёт.
        assertThat(stats.matches()).isZero();
        assertThat(stats.streak().kind()).isEqualTo("NONE");
    }

    @DisplayName("Should count the streak by places When the rating went the other way")
    @Test
    void shouldCountTheStreakByPlacesWhenTheRatingWentTheOtherWay() {
        when(matchPlayers.findByUserId(PLAYER)).thenReturn(List.of(finished(1, null)));
        when(matches.findAllById(any())).thenReturn(List.of());
        // ⭐ Первое место, но рейтинг упал: соперники были слабее, Elo это учитывает.
        // Серия обязана считаться по местам — за столом помнят, кто был первым.
        when(histories.findByUserIdOrderByCreatedAtDesc(PLAYER))
                .thenReturn(List.of(ratingRow(1, 1000, 995)));

        final StatsDtos.PlayerStats stats = service().of(PLAYER);

        assertThat(stats.streak().kind()).isEqualTo("WIN");
        assertThat(stats.streak().length()).isEqualTo(1);
    }

    @DisplayName("Should stop the streak When the last matches went differently")
    @Test
    void shouldStopTheStreakWhenTheLastMatchesWentDifferently() {
        when(matchPlayers.findByUserId(PLAYER))
                .thenReturn(List.of(finished(1, null), finished(2, LossDegree.FAIL)));
        when(matches.findAllById(any())).thenReturn(List.of());
        when(histories.findByUserIdOrderByCreatedAtDesc(PLAYER)).thenReturn(List.of(
                ratingRow(1, 980, 1000),   // последний матч — победа
                ratingRow(2, 1000, 980),   // до него — проигрыш
                ratingRow(2, 1020, 1000)));

        final StatsDtos.PlayerStats stats = service().of(PLAYER);

        assertThat(stats.streak().kind()).isEqualTo("WIN");
        assertThat(stats.streak().length()).isEqualTo(1);
        assertThat(stats.avgPlace()).isEqualByComparingTo(BigDecimal.valueOf(1.5));
    }

    @DisplayName("Should order the degrees from the heaviest When losses differ")
    @Test
    void shouldOrderTheDegreesFromTheHeaviestWhenLossesDiffer() {
        when(matchPlayers.findByUserId(PLAYER)).thenReturn(List.of(
                finished(2, LossDegree.FAIL),
                finished(2, LossDegree.ROYAL),
                finished(2, LossDegree.FAIL)));
        when(matches.findAllById(any())).thenReturn(List.of());
        when(histories.findByUserIdOrderByCreatedAtDesc(PLAYER)).thenReturn(List.of());

        final StatsDtos.PlayerStats stats = service().of(PLAYER);

        // Степени объявлены от тяжёлой к обычной (§0.3) — в таком порядке их и показывают.
        assertThat(stats.degrees()).extracting(StatsDtos.DegreeCount::degree)
                .containsExactly("ROYAL", "FAIL");
        assertThat(stats.degrees().get(1).count()).isEqualTo(2);
        assertThat(stats.losses()).isEqualTo(3);
    }

    private StatsService service() {
        return new StatsService(matchPlayers, matches, histories);
    }

    private MatchPlayerRecord unfinished() {
        return new MatchPlayerRecord(UUID.randomUUID(), PLAYER, 0);
    }

    private MatchPlayerRecord finished(final int place, final LossDegree degree) {
        final MatchPlayerRecord row = new MatchPlayerRecord(UUID.randomUUID(), PLAYER, 0);
        row.finish(place, "8", degree, BigDecimal.valueOf(1000), BigDecimal.valueOf(1010));
        return row;
    }

    private RatingHistoryEntry ratingRow(final int place, final int before, final int after) {
        return new RatingHistoryEntry(PLAYER, UUID.randomUUID(), BigDecimal.valueOf(before),
                BigDecimal.valueOf(after), BigDecimal.valueOf(350), place, 2, null);
    }
}
