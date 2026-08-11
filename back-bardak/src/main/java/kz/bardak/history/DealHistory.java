package kz.bardak.history;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ArrayNode;
import java.time.Clock;
import java.util.List;
import java.util.Objects;
import java.util.UUID;
import kz.bardak.game.protocol.CardCodec;
import kz.bardak.game.protocol.NavesLevelCodec;
import kz.bardak.game.rules.Card;
import kz.bardak.game.rules.DealOutcome;
import kz.bardak.game.rules.LevelChange;
import kz.bardak.game.rules.NavesScale;
import kz.bardak.game.rules.PlayerOutcome;
import kz.bardak.history.domain.DealRecord;
import kz.bardak.history.domain.DealRepository;
import kz.bardak.history.domain.DealResultRecord;
import kz.bardak.history.domain.DealResultRepository;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

/**
 * Запись сыгранной раздачи.
 *
 * <p>⭐ Пишется итог, а не ход раздачи: ходы уже лежат в логе событий. Здесь — то, что
 * иначе пришлось бы восстанавливать переигрыванием: козырь, состав последней атаки, места,
 * навешенное и из чего сложился сдвиг уровня.
 */
@Service
public class DealHistory {

    private static final Logger log = LoggerFactory.getLogger(DealHistory.class);

    private final DealRepository deals;
    private final DealResultRepository results;
    private final ObjectMapper objectMapper;
    private final Clock clock;

    public DealHistory(final DealRepository deals, final DealResultRepository results,
                       final ObjectMapper objectMapper, final Clock clock) {
        this.deals = Objects.requireNonNull(deals, "deals");
        this.results = Objects.requireNonNull(results, "results");
        this.objectMapper = Objects.requireNonNull(objectMapper, "objectMapper");
        this.clock = Objects.requireNonNull(clock, "clock");
    }

    @Transactional
    public void record(final UUID matchId, final int dealNo, final DealOutcome outcome,
                       final NavesScale scale) {
        if (deals.existsByMatchIdAndDealNo(matchId, (short) dealNo)) {
            log.warn("Раздача {} матча {} уже записана, пропускаю", dealNo, matchId);
            return;
        }
        final DealRecord deal = deals.save(new DealRecord(UUID.randomUUID(), matchId, dealNo,
                outcome.trumpSuit() == null ? null : outcome.trumpSuit().name(),
                outcome.dealLoserSeat(), cardsJson(outcome.lastAttackCards()), clock.instant()));
        for (final PlayerOutcome player : outcome.players()) {
            results.save(new DealResultRecord(deal.id(), player.seatNo(), player.place(),
                    cardsJson(player.hungCards()),
                    NavesLevelCodec.encode(scale, player.levelBefore()),
                    NavesLevelCodec.encode(scale, player.levelAfter()),
                    changesJson(player.changes())));
        }
    }

    private String cardsJson(final List<Card> cards) {
        final ArrayNode array = objectMapper.createArrayNode();
        cards.forEach(card -> array.add(CardCodec.encode(card)));
        return array.toString();
    }

    private String changesJson(final List<LevelChange> changes) {
        final ArrayNode array = objectMapper.createArrayNode();
        for (final LevelChange change : changes) {
            array.add(objectMapper.createObjectNode()
                    .put("reason", change.reason().name())
                    .put("amount", change.amount()));
        }
        return array.toString();
    }
}
