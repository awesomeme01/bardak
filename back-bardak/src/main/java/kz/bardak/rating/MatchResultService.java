package kz.bardak.rating;

import java.math.BigDecimal;
import java.math.RoundingMode;
import java.time.Clock;
import java.util.ArrayList;
import java.util.Collections;
import java.util.Comparator;
import java.util.List;
import java.util.Objects;
import java.util.UUID;
import kz.bardak.game.protocol.NavesLevelCodec;
import kz.bardak.game.rules.DealOutcome;
import kz.bardak.game.rules.LossDegree;
import kz.bardak.game.rules.MatchState;
import kz.bardak.game.rules.NavesScale;
import kz.bardak.game.rules.PlayerOutcome;
import kz.bardak.history.MatchLog;
import kz.bardak.history.domain.MatchPlayerRecord;
import kz.bardak.history.domain.MatchPlayerRepository;
import kz.bardak.rating.domain.RatingHistoryEntry;
import kz.bardak.rating.domain.RatingHistoryRepository;
import kz.bardak.rating.domain.Season;
import kz.bardak.rating.domain.SeasonRepository;
import kz.bardak.rating.domain.UserRating;
import kz.bardak.rating.domain.UserRatingRepository;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

/**
 * Запись итога матча и пересчёт рейтинга.
 *
 * <p>⭐ Всё в <b>одной транзакции</b>: «матч записался, а рейтинг нет» даёт неисправимое
 * расхождение между историей и текущим рейтингом — починить его потом можно только руками
 * и на глаз.
 *
 * <p>Отменённый матч (`ABORTED`) сюда не приходит вообще: он сохраняется для просмотра,
 * но рейтинга не касается (`07-rating-system.md`).
 */
@Service
public class MatchResultService {

    private static final Logger log = LoggerFactory.getLogger(MatchResultService.class);

    private final UserRatingRepository ratings;
    private final RatingHistoryRepository histories;
    private final SeasonRepository seasons;
    private final MatchPlayerRepository matchPlayers;
    private final MatchLog matchLog;
    private final Clock clock;

    public MatchResultService(final UserRatingRepository ratings,
                              final RatingHistoryRepository histories,
                              final SeasonRepository seasons,
                              final MatchPlayerRepository matchPlayers, final MatchLog matchLog,
                              final Clock clock) {
        this.ratings = Objects.requireNonNull(ratings, "ratings");
        this.histories = Objects.requireNonNull(histories, "histories");
        this.seasons = Objects.requireNonNull(seasons, "seasons");
        this.matchPlayers = Objects.requireNonNull(matchPlayers, "matchPlayers");
        this.matchLog = Objects.requireNonNull(matchLog, "matchLog");
        this.clock = Objects.requireNonNull(clock, "clock");
    }

    /** Посадить игроков: места известны при старте, итог — нет. */
    @Transactional
    public void startMatch(final UUID matchId, final List<UUID> seatUsers) {
        for (int seat = 0; seat < seatUsers.size(); seat++) {
            matchPlayers.save(new MatchPlayerRecord(matchId, seatUsers.get(seat), seat));
        }
    }

    /**
     * Завершить матч: места, уровни навесов, степени проигрыша и рейтинг — одной транзакцией.
     *
     * @param seatUsers игроки по местам движка
     * @param state     состояние законченного матча
     * @param scale     шкала навесов этого стола: уровень пишется её кодами
     * @return изменения рейтинга по местам — их показывает экран итога
     */
    @Transactional
    public List<RatingChange> finishMatch(final UUID matchId, final List<UUID> seatUsers,
                                          final MatchState state, final NavesScale scale) {
        if (histories.existsByMatchId(matchId)) {
            // Повторный вызов не должен удваивать рейтинг: матч закрывается ровно один раз.
            log.warn("Матч {} уже посчитан, пропускаю", matchId);
            return List.of();
        }
        final DealOutcome outcome = state.lastResult().orElseThrow(
                () -> new IllegalStateException("Матч закончился без итога раздачи"));
        final List<Integer> places = placesOf(outcome, seatUsers.size());
        final UUID seasonId = seasons.findFirstByClosedAtIsNull().map(Season::id).orElse(null);

        final List<UserRating> current = new ArrayList<>();
        final List<EloRating.Participant> participants = new ArrayList<>();
        for (int seat = 0; seat < seatUsers.size(); seat++) {
            final UserRating rating = ratingOf(seatUsers.get(seat));
            current.add(rating);
            participants.add(new EloRating.Participant(rating.rating().doubleValue(),
                    rating.matchesPlayed(), places.get(seat)));
        }
        final List<Double> updated = EloRating.recalculate(participants);

        final List<RatingChange> changes = new ArrayList<>();
        for (int seat = 0; seat < seatUsers.size(); seat++) {
            final int seatNo = seat;
            final UUID userId = seatUsers.get(seat);
            final PlayerOutcome result = outcome.forSeat(seat);
            final UserRating rating = current.get(seat);
            final BigDecimal before = rating.rating();
            final BigDecimal after = BigDecimal.valueOf(updated.get(seat))
                    .setScale(2, RoundingMode.HALF_UP);
            rating.apply(after, clock.instant());
            ratings.save(rating);
            histories.save(new RatingHistoryEntry(userId, matchId, before, after,
                    rating.deviation(), places.get(seat), seatUsers.size(), seasonId));

            final String level = NavesLevelCodec.encode(scale, result.levelAfter());
            matchPlayers.findById(new MatchPlayerRecord.Key(matchId, userId))
                    // Строка обычно уже есть со старта матча; создаётся здесь только если
                    // матч восстановлен из снимка после рестарта.
                    .orElseGet(() -> matchPlayers.save(new MatchPlayerRecord(matchId, userId, seatNo)))
                    .finish(places.get(seat), level, result.lossDegree(), before, after);
            changes.add(new RatingChange(userId, places.get(seat), level, result.lossDegree(),
                    before, after));
        }

        matchLog.finish(matchId, outcome.mainLoser()
                .map(loser -> seatUsers.get(loser.seatNo()))
                .orElse(null));
        return List.copyOf(changes);
    }

    /**
     * Места по итогу матча.
     *
     * <p>⭐ Место определяет уровень навесов: чем ниже уровень, тем лучше сыграл — счёта в
     * очках в игре нет (ADR-017). Проигравшие с джокером идут в конец и ранжируются между
     * собой по степени: {@code ROYAL} тяжелее, чем {@code FAIL} (§0.3).
     *
     * <p>Равные уровни делят соседние места по порядку мест за столом. Делёж мест Elo не
     * предусматривает, а придумывать ради этого отдельное правило незачем: на итог влияет
     * только то, кто выше кого.
     */
    private List<Integer> placesOf(final DealOutcome outcome, final int seats) {
        final List<Integer> order = new ArrayList<>();
        for (int seat = 0; seat < seats; seat++) {
            order.add(seat);
        }
        // Степени объявлены от тяжёлой к обычной, поэтому здесь порядок обратный:
        // тяжёлая степень — худшее место, а не лучшее.
        order.sort(Comparator.comparingInt((Integer seat) -> outcome.forSeat(seat).levelAfter())
                .thenComparing(seat -> outcome.forSeat(seat).degree().orElse(null),
                        Comparator.nullsFirst(Comparator.reverseOrder()))
                .thenComparingInt(seat -> seat));

        final List<Integer> places = new ArrayList<>(Collections.nCopies(seats, 0));
        for (int index = 0; index < order.size(); index++) {
            places.set(order.get(index), index + 1);
        }
        return places;
    }

    private UserRating ratingOf(final UUID userId) {
        return ratings.findById(userId)
                .orElseGet(() -> ratings.save(new UserRating(userId, clock.instant())));
    }

    /** Что показать игроку после матча. */
    public record RatingChange(UUID userId, int place, String navesLevel, LossDegree lossDegree,
                               BigDecimal before, BigDecimal after) {

        public BigDecimal delta() {
            return after.subtract(before);
        }
    }
}
