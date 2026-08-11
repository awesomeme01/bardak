package kz.bardak.history;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.math.BigDecimal;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.UUID;
import java.util.function.Function;
import java.util.stream.Collectors;
import kz.bardak.auth.domain.User;
import kz.bardak.auth.domain.UserRepository;
import kz.bardak.common.web.ApiException;
import kz.bardak.history.api.HistoryDtos;
import kz.bardak.history.domain.DealRecord;
import kz.bardak.history.domain.DealRepository;
import kz.bardak.history.domain.DealResultRecord;
import kz.bardak.history.domain.DealResultRepository;
import kz.bardak.history.domain.MatchEventRecord;
import kz.bardak.history.domain.MatchPlayerRecord;
import kz.bardak.history.domain.MatchPlayerRepository;
import kz.bardak.history.domain.MatchRecord;
import kz.bardak.history.domain.MatchRecordRepository;
import kz.bardak.history.domain.MatchRecordStatus;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

/**
 * История матчей: список, детали, разбивка по раздачам и реплей.
 *
 * <p>⭐ Реплей отдаётся <b>с точки зрения спрашивающего</b>, а не целиком. Лог хранит и то,
 * что видел один игрок, — отдать его сырым значит показать чужие карты задним числом,
 * а по ним читается стиль игры соперника не хуже, чем подглядыванием вживую.
 */
@Service
public class MatchHistoryService {

    private final MatchRecordRepository matches;
    private final MatchPlayerRepository matchPlayers;
    private final DealRepository deals;
    private final DealResultRepository dealResults;
    private final MatchLog matchLog;
    private final UserRepository users;
    private final ObjectMapper objectMapper;

    public MatchHistoryService(final MatchRecordRepository matches,
                               final MatchPlayerRepository matchPlayers, final DealRepository deals,
                               final DealResultRepository dealResults, final MatchLog matchLog,
                               final UserRepository users, final ObjectMapper objectMapper) {
        this.matches = Objects.requireNonNull(matches, "matches");
        this.matchPlayers = Objects.requireNonNull(matchPlayers, "matchPlayers");
        this.deals = Objects.requireNonNull(deals, "deals");
        this.dealResults = Objects.requireNonNull(dealResults, "dealResults");
        this.matchLog = Objects.requireNonNull(matchLog, "matchLog");
        this.users = Objects.requireNonNull(users, "users");
        this.objectMapper = Objects.requireNonNull(objectMapper, "objectMapper");
    }

    /** Матчи игрока, новые сверху. */
    @Transactional(readOnly = true)
    public List<HistoryDtos.MatchSummary> matchesOf(final UUID userId, final String status) {
        final List<UUID> played = matchPlayers.findByUserId(userId).stream()
                .map(MatchPlayerRecord::matchId)
                .toList();
        if (played.isEmpty()) {
            return List.of();
        }
        return matches.findByIdInOrderByStartedAtDesc(played).stream()
                .filter(match -> status == null || match.status().name().equalsIgnoreCase(status))
                .map(match -> summaryOf(match, userId))
                .toList();
    }

    @Transactional(readOnly = true)
    public HistoryDtos.MatchDetails details(final UUID matchId, final UUID userId) {
        final MatchRecord match = matchOf(matchId);
        final List<HistoryDtos.DealSummary> played = dealsOf(matchId);
        return new HistoryDtos.MatchDetails(summaryOf(match, userId), played);
    }

    @Transactional(readOnly = true)
    public List<HistoryDtos.DealSummary> dealsOf(final UUID matchId) {
        final List<HistoryDtos.DealSummary> summaries = new ArrayList<>();
        for (final DealRecord deal : deals.findByMatchIdOrderByDealNo(matchId)) {
            final List<HistoryDtos.DealSeatResult> seats = dealResults
                    .findByDealIdOrderBySeatNo(deal.id()).stream()
                    .map(this::toSeatResult)
                    .toList();
            summaries.add(new HistoryDtos.DealSummary(deal.dealNo(), deal.trumpSuit(),
                    deal.loserSeat(), deal.finishedAt(), stringList(deal.lastAttackCards()), seats));
        }
        return summaries;
    }

    /**
     * Реплей матча.
     *
     * <p>⚠️ Только для законченных матчей: у идущего лог — это и есть текущая партия,
     * и «посмотреть реплей» посреди неё означало бы читать её из другого окна.
     */
    @Transactional(readOnly = true)
    public HistoryDtos.Replay replay(final UUID matchId, final UUID userId) {
        final MatchRecord match = matchOf(matchId);
        if (match.status() == MatchRecordStatus.IN_PROGRESS
                || match.status() == MatchRecordStatus.PAUSED) {
            throw new ApiException(HttpStatus.CONFLICT, "MATCH_NOT_FINISHED",
                    "Реплей доступен только после матча");
        }
        final int seat = seatOf(matchId, userId);
        final List<HistoryDtos.ReplayEvent> events = matchLog.since(matchId, 0, seat).stream()
                .map(this::toReplayEvent)
                .toList();
        return new HistoryDtos.Replay(matchId, match.status().name(), seat, events);
    }

    private HistoryDtos.MatchSummary summaryOf(final MatchRecord match, final UUID userId) {
        final List<MatchPlayerRecord> seats = matchPlayers.findByMatchIdOrderBySeatNo(match.id());
        final Map<UUID, String> names = users.findAllById(seats.stream()
                        .map(MatchPlayerRecord::userId).toList()).stream()
                .collect(Collectors.toMap(User::id, User::displayName));
        final List<HistoryDtos.PlayerResult> players = seats.stream()
                .map(seat -> new HistoryDtos.PlayerResult(seat.userId(),
                        names.getOrDefault(seat.userId(), "—"), seat.seatNo(), seat.place(),
                        seat.navesLevel(), seat.lossType() == null ? null : seat.lossType().name(),
                        seat.ratingBefore(), seat.ratingAfter(), seat.ratingDelta()))
                .sorted(Comparator.comparing(HistoryDtos.PlayerResult::place,
                        Comparator.nullsLast(Comparator.naturalOrder())))
                .toList();
        final MatchPlayerRecord mine = seats.stream()
                .filter(seat -> seat.userId().equals(userId))
                .findFirst()
                .orElse(null);
        return new HistoryDtos.MatchSummary(match.id(), match.tableId(), match.status().name(),
                match.startedAt(), match.finishedAt(), match.playersCount(), match.dealsPlayed(),
                match.abortReason(), match.status() == MatchRecordStatus.FINISHED,
                mine == null ? null : mine.place(),
                mine == null ? null : mine.ratingDelta(), players);
    }

    private HistoryDtos.DealSeatResult toSeatResult(final DealResultRecord result) {
        final List<HistoryDtos.LevelChangeView> changes = new ArrayList<>();
        for (final JsonNode change : readTree(result.levelChanges())) {
            changes.add(new HistoryDtos.LevelChangeView(change.get("reason").asText(),
                    change.get("amount").asInt()));
        }
        return new HistoryDtos.DealSeatResult(result.seatNo(), result.place(),
                stringList(result.hungCards()), result.navesLevelBefore(),
                result.navesLevelAfter(), changes);
    }

    private HistoryDtos.ReplayEvent toReplayEvent(final MatchEventRecord event) {
        return new HistoryDtos.ReplayEvent(event.seq(), event.dealNo(), event.type(),
                event.actorSeat(), readTree(event.payload()));
    }

    /** Место спрашивающего или {@code -1}, если он в матче не играл: тогда видно публичное. */
    private int seatOf(final UUID matchId, final UUID userId) {
        return matchPlayers.findByMatchIdOrderBySeatNo(matchId).stream()
                .filter(seat -> seat.userId().equals(userId))
                .mapToInt(MatchPlayerRecord::seatNo)
                .findFirst()
                .orElse(-1);
    }

    private MatchRecord matchOf(final UUID matchId) {
        return matches.findById(matchId).orElseThrow(() ->
                new ApiException(HttpStatus.NOT_FOUND, "MATCH_NOT_FOUND", "Такого матча нет"));
    }

    private List<String> stringList(final String json) {
        final List<String> values = new ArrayList<>();
        readTree(json).forEach(node -> values.add(node.asText()));
        return values;
    }

    private JsonNode readTree(final String json) {
        try {
            return objectMapper.readTree(json);
        } catch (final JsonProcessingException e) {
            throw new IllegalStateException("В базе лежит неразбираемый JSON", e);
        }
    }
}
