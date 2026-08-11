package kz.bardak.history.api;

import com.fasterxml.jackson.databind.JsonNode;
import java.math.BigDecimal;
import java.time.Instant;
import java.util.List;
import java.util.UUID;

/** Формы истории матчей (`05-api-contracts.md`). */
public final class HistoryDtos {

    private HistoryDtos() {
    }

    /**
     * Матч в списке.
     *
     * @param ratingCounted ⭐ отменённый матч виден в истории, но рейтинга не касается —
     *                      без этого признака нулевая дельта выглядела бы как ничья
     */
    public record MatchSummary(UUID id, UUID tableId, String status, Instant startedAt,
                               Instant finishedAt, int playersCount, int dealsPlayed,
                               String abortReason, boolean ratingCounted, Integer myPlace,
                               BigDecimal myRatingDelta, List<PlayerResult> players) {
    }

    public record PlayerResult(UUID userId, String displayName, int seatNo, Integer place,
                               String navesLevel, String lossType, BigDecimal ratingBefore,
                               BigDecimal ratingAfter, BigDecimal ratingDelta) {
    }

    public record MatchDetails(MatchSummary match, List<DealSummary> deals) {
    }

    public record DealSummary(int dealNo, String trumpSuit, int loserSeat, Instant finishedAt,
                              List<String> lastAttackCards, List<DealSeatResult> seats) {
    }

    public record DealSeatResult(int seatNo, Integer place, List<String> hungCards,
                                 String navesLevelBefore, String navesLevelAfter,
                                 List<LevelChangeView> levelChanges) {
    }

    public record LevelChangeView(String reason, int amount) {
    }

    /** Одно событие реплея — ровно в той форме, в какой оно уходило живьём. */
    public record ReplayEvent(int seq, Integer dealNo, String type, Integer actorSeat,
                              JsonNode payload) {
    }

    public record Replay(UUID matchId, String status, int mySeat, List<ReplayEvent> events) {
    }
}
