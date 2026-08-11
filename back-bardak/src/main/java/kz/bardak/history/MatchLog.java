package kz.bardak.history;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.time.Clock;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.UUID;
import kz.bardak.game.protocol.GameProtocol;
import kz.bardak.game.rules.DealEvent;
import kz.bardak.history.domain.MatchEventRecord;
import kz.bardak.history.domain.MatchEventRepository;
import kz.bardak.history.domain.MatchRecord;
import kz.bardak.history.domain.MatchRecordRepository;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

/**
 * Запись матча в лог (ADR-004).
 *
 * <p>⭐ Событие пишется <b>до</b> рассылки клиентам. Порядок именно такой, а не наоборот:
 * иначе после падения между рассылкой и записью клиенты видели бы ход, которого в истории
 * нет, — и реплей разошёлся бы с тем, что люди видели своими глазами.
 *
 * <p>⚠️ В лог уходит полная информация, включая скрытую: какая карта у кого. Это внутренняя
 * запись, наружу она отдаётся только через проекцию.
 */
@Service
public class MatchLog {

    private final MatchRecordRepository matches;
    private final MatchEventRepository events;
    private final ObjectMapper objectMapper;
    private final Clock clock;

    public MatchLog(final MatchRecordRepository matches, final MatchEventRepository events,
                    final ObjectMapper objectMapper, final Clock clock) {
        this.matches = Objects.requireNonNull(matches, "matches");
        this.events = Objects.requireNonNull(events, "events");
        this.objectMapper = Objects.requireNonNull(objectMapper, "objectMapper");
        this.clock = Objects.requireNonNull(clock, "clock");
    }

    @Transactional
    public MatchRecord startMatch(final UUID tableId, final int playersCount, final long rngSeed,
                                  final String rulesSnapshot) {
        return matches.save(new MatchRecord(UUID.randomUUID(), tableId, playersCount, rngSeed,
                rulesSnapshot));
    }

    /**
     * Дописать события хода.
     *
     * @param firstSeq номер первого события; нумерация сквозная по матчу и начинается с 1
     * @return номер последнего записанного события
     */
    @Transactional
    public int append(final UUID matchId, final int firstSeq, final int dealNo,
                      final List<DealEvent> dealEvents) {
        int seq = firstSeq;
        for (final DealEvent event : dealEvents) {
            events.save(new MatchEventRecord(matchId, seq, dealNo, GameProtocol.eventType(event),
                    event.seatNo(), asJson(GameProtocol.toEventPayload(event)),
                    event.privateToSeat().orElse(null)));
            seq++;
        }
        return seq - 1;
    }

    /**
     * ⭐ Отклонённая попытка хода тоже пишется: она часть истории стола, хотя состояние
     * не меняет (§2.1). Без неё в логе не видно, что человек вообще дёргался.
     */
    @Transactional
    public void appendRejected(final UUID matchId, final int seq, final int dealNo, final int actorSeat,
                               final String commandType, final String reason) {
        // Попытку видит только тот, кто её сделал: остальным полагается лишь факт
        // и рубашка (§2.1), а какая именно карта — не покидает сервер.
        events.save(new MatchEventRecord(matchId, seq, dealNo, "MOVE_REJECTED", actorSeat,
                asJson(Map.of("command", commandType, "reason", reason)), actorSeat));
    }

    /**
     * События, пропущенные клиентом, — уже отфильтрованные по видимости.
     *
     * <p>⭐ Сырой лог наружу не отдаётся никогда: в нём лежит скрытая информация. Фильтр
     * идёт по записанной вместе с событием видимости, а не по пересчёту правил — иначе
     * правило жило бы в двух местах и однажды разошлось.
     */
    @Transactional(readOnly = true)
    public List<MatchEventRecord> since(final UUID matchId, final int lastSeq, final int seatNo) {
        return events.findByMatchIdAndSeqGreaterThanOrderBySeqAsc(matchId, lastSeq).stream()
                .filter(event -> event.isVisibleTo(seatNo))
                .toList();
    }

    @Transactional
    public void dealsPlayed(final UUID matchId, final int dealsPlayed) {
        matches.findById(matchId).ifPresent(match -> {
            match.dealsPlayed(dealsPlayed);
            matches.save(match);
        });
    }

    @Transactional
    public void finish(final UUID matchId, final UUID loserUserId) {
        matches.findById(matchId).ifPresent(match -> {
            match.finish(loserUserId, clock.instant());
            matches.save(match);
        });
    }

    @Transactional
    public void abort(final UUID matchId, final String reason) {
        matches.findById(matchId).ifPresent(match -> {
            match.abort(reason, clock.instant());
            matches.save(match);
        });
    }

    private String asJson(final Map<String, Object> payload) {
        try {
            return objectMapper.writeValueAsString(payload);
        } catch (final JsonProcessingException e) {
            throw new IllegalStateException("Событие лога не сериализуется", e);
        }
    }
}
