package kz.bardak.rating;

import java.math.BigDecimal;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.EnumMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.UUID;
import kz.bardak.game.rules.LossDegree;
import kz.bardak.history.domain.MatchPlayerRecord;
import kz.bardak.history.domain.MatchPlayerRepository;
import kz.bardak.history.domain.MatchRecord;
import kz.bardak.history.domain.MatchRecordRepository;
import kz.bardak.history.domain.MatchRecordStatus;
import kz.bardak.rating.api.StatsDtos;
import kz.bardak.rating.domain.RatingHistoryEntry;
import kz.bardak.rating.domain.RatingHistoryRepository;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

/**
 * Статистика игрока.
 *
 * <p>⭐ Считается по уже записанному, а не копится отдельными счётчиками. Счётчик — это
 * второе место, где живёт правда: он неизбежно разъезжается с историей, и потом не понять,
 * какое из двух чисел настоящее. История матчей есть, и её достаточно.
 *
 * <p>⚠️ Считается на лету по всем матчам игрока. Для узкого круга это десятки строк;
 * когда станет тысячами — сюда придёт кэш или витрина, а не досчитывание в живых запросах.
 */
@Service
public class StatsService {

    private final MatchPlayerRepository matchPlayers;
    private final MatchRecordRepository matches;
    private final RatingHistoryRepository histories;

    public StatsService(final MatchPlayerRepository matchPlayers,
                        final MatchRecordRepository matches,
                        final RatingHistoryRepository histories) {
        this.matchPlayers = Objects.requireNonNull(matchPlayers, "matchPlayers");
        this.matches = Objects.requireNonNull(matches, "matches");
        this.histories = Objects.requireNonNull(histories, "histories");
    }

    @Transactional(readOnly = true)
    public StatsDtos.PlayerStats of(final UUID userId) {
        final List<MatchPlayerRecord> played = matchPlayers.findByUserId(userId).stream()
                .filter(row -> row.place() != null)   // отменённые матчи в счёт не идут (§5.3)
                .toList();
        if (played.isEmpty()) {
            return StatsDtos.PlayerStats.empty();
        }

        final Map<UUID, MatchRecord> byId = new java.util.HashMap<>();
        matches.findAllById(played.stream().map(MatchPlayerRecord::matchId).toList())
                .forEach(match -> byId.put(match.id(), match));

        int wins = 0;
        int losses = 0;
        int places = 0;
        int deals = 0;
        final Map<LossDegree, Integer> degrees = new EnumMap<>(LossDegree.class);
        for (final MatchPlayerRecord row : played) {
            places += row.place();
            if (row.place() == 1) {
                wins++;
            }
            if (row.lossType() != null) {
                losses++;
                degrees.merge(row.lossType(), 1, Integer::sum);
            }
            final MatchRecord match = byId.get(row.matchId());
            if (match != null && match.status() == MatchRecordStatus.FINISHED) {
                deals += match.dealsPlayed();
            }
        }

        final List<RatingHistoryEntry> ratings = histories.findByUserIdOrderByCreatedAtDesc(userId);
        return new StatsDtos.PlayerStats(
                played.size(),
                wins,
                losses,
                round((double) places / played.size()),
                deals,
                streakOf(ratings),
                ratings.stream().map(RatingHistoryEntry::ratingAfter)
                        .max(Comparator.naturalOrder()).orElse(null),
                ratings.stream().map(RatingHistoryEntry::ratingAfter)
                        .min(Comparator.naturalOrder()).orElse(null),
                degreeList(degrees));
    }

    /**
     * Текущая серия: сколько подряд выиграно или проиграно.
     *
     * <p>⭐ Серия считается по местам, а не по знаку рейтинга: в матче на пятерых можно
     * занять второе место и всё равно потерять очки. Побеждает тот, кто первый, — это
     * и есть исход, который помнят за столом.
     */
    private StatsDtos.Streak streakOf(final List<RatingHistoryEntry> ratings) {
        if (ratings.isEmpty()) {
            return new StatsDtos.Streak("NONE", 0);
        }
        final boolean won = ratings.get(0).place() == 1;
        int length = 0;
        for (final RatingHistoryEntry entry : ratings) {
            if ((entry.place() == 1) != won) {
                break;
            }
            length++;
        }
        return new StatsDtos.Streak(won ? "WIN" : "LOSS", length);
    }

    private List<StatsDtos.DegreeCount> degreeList(final Map<LossDegree, Integer> degrees) {
        final List<StatsDtos.DegreeCount> list = new ArrayList<>();
        // Порядок объявления — от самой тяжёлой степени к обычной (§0.3), он же и нужен.
        for (final LossDegree degree : LossDegree.values()) {
            final int count = degrees.getOrDefault(degree, 0);
            if (count > 0) {
                list.add(new StatsDtos.DegreeCount(degree.name(), count));
            }
        }
        return list;
    }

    private BigDecimal round(final double value) {
        return BigDecimal.valueOf(value).setScale(2, java.math.RoundingMode.HALF_UP);
    }
}
