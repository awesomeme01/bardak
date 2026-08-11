package kz.bardak.game.protocol;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ArrayNode;
import com.fasterxml.jackson.databind.node.ObjectNode;
import java.util.ArrayList;
import java.util.List;
import java.util.Objects;
import kz.bardak.game.rules.Card;
import kz.bardak.game.rules.DealOutcome;
import kz.bardak.game.rules.LevelChange;
import kz.bardak.game.rules.LevelChangeReason;
import kz.bardak.game.rules.DealPhase;
import kz.bardak.game.rules.DealState;
import kz.bardak.game.rules.HangClaim;
import kz.bardak.game.rules.HangingWindow;
import kz.bardak.game.rules.LossDegree;
import kz.bardak.game.rules.MatchPhase;
import kz.bardak.game.rules.MatchState;
import kz.bardak.game.rules.PendingHiddenTrump;
import kz.bardak.game.rules.PlayerOutcome;
import kz.bardak.game.rules.PlayerState;
import kz.bardak.game.rules.Suit;
import kz.bardak.game.rules.TableSlot;
import kz.bardak.game.rules.Trump;

/**
 * Снимок состояния матча в JSON и обратно.
 *
 * <p>⭐ Кодек написан руками, а не через аннотации Jackson на записях движка. Причина
 * принципиальная: {@code game.rules} не знает ни про Spring, ни про JSON, ни про базу
 * (`02-architecture.md`), и разметить его аннотациями значит впустить инфраструктуру
 * в единственное место, которое от неё намеренно свободно.
 *
 * <p>Цена — этот файл. Она оплачивается тестом на круговой прогон: снимок, разобранный
 * обратно, обязан совпасть с исходным состоянием до последнего поля.
 */
public final class MatchStateCodec {

    private final ObjectMapper objectMapper;

    public MatchStateCodec(final ObjectMapper objectMapper) {
        this.objectMapper = Objects.requireNonNull(objectMapper, "objectMapper");
    }

    public String encode(final MatchState state) {
        Objects.requireNonNull(state, "state");
        final ObjectNode root = objectMapper.createObjectNode();
        root.put("phase", state.phase().name());
        root.put("dealNo", state.dealNo());
        root.put("matchSeed", state.matchSeed());
        root.set("navesLevels", intArray(state.navesLevels()));
        root.set("deal", encodeDeal(state.deal()));
        root.set("results", encodeResults(state.results()));
        return root.toString();
    }

    public MatchState decode(final String json) {
        try {
            final JsonNode root = objectMapper.readTree(json);
            return new MatchState(
                    MatchPhase.valueOf(root.get("phase").asText()),
                    intList(root.get("navesLevels")),
                    root.get("dealNo").asInt(),
                    root.get("matchSeed").asLong(),
                    decodeDeal(root.get("deal")),
                    decodeResults(root.get("results")));
        } catch (final com.fasterxml.jackson.core.JsonProcessingException e) {
            throw new IllegalStateException("Снимок матча не разбирается", e);
        }
    }

    private ObjectNode encodeDeal(final DealState deal) {
        final ObjectNode node = objectMapper.createObjectNode();
        node.put("phase", deal.phase().name());
        if (deal.hasTrump()) {
            node.put("trump", deal.trump().suit().name());
        }
        node.set("deck", cardArray(deal.deck()));
        final ArrayNode players = objectMapper.createArrayNode();
        for (final PlayerState player : deal.players()) {
            players.add(encodePlayer(player));
        }
        node.set("players", players);
        final ArrayNode table = objectMapper.createArrayNode();
        for (final TableSlot slot : deal.table()) {
            final ObjectNode slotNode = objectMapper.createObjectNode();
            slotNode.put("attack", CardCodec.encode(slot.attack()));
            slot.defenceCard().ifPresent(card -> slotNode.put("defence", CardCodec.encode(card)));
            table.add(slotNode);
        }
        node.set("table", table);
        node.put("roundStarterSeat", deal.roundStarterSeat());
        node.put("attackRightSeat", deal.attackRightSeat());
        node.put("defenderSeat", deal.defenderSeat());
        node.set("passedSeats", intArray(deal.passedSeats()));
        node.set("exitOrder", intArray(deal.exitOrder()));
        node.put("anyCardBeatenThisRound", deal.anyCardBeatenThisRound());
        node.put("anyPileDiscarded", deal.anyPileDiscarded());
        node.set("lastAttackCards", cardArray(deal.lastAttackCards()));
        node.put("rngSeed", deal.rngSeed());
        node.put("diceRolls", deal.diceRolls());
        deal.hanging().ifPresent(window -> node.set("hangingWindow", encodeWindow(window)));
        deal.hiddenTrumpAwaitingSuit().ifPresent(pending -> {
            final ObjectNode pendingNode = objectMapper.createObjectNode();
            pendingNode.put("card", CardCodec.encode(pending.card()));
            pendingNode.put("recipientSeat", pending.recipientSeat());
            pendingNode.put("chooserSeat", pending.chooserSeat());
            node.set("pendingHiddenTrump", pendingNode);
        });
        return node;
    }

    private DealState decodeDeal(final JsonNode node) {
        final List<PlayerState> players = new ArrayList<>();
        for (final JsonNode player : node.get("players")) {
            players.add(decodePlayer(player));
        }
        final List<TableSlot> table = new ArrayList<>();
        for (final JsonNode slot : node.get("table")) {
            final Card attack = CardCodec.decode(slot.get("attack").asText());
            table.add(slot.hasNonNull("defence")
                    ? TableSlot.of(attack).beatenWith(CardCodec.decode(slot.get("defence").asText()))
                    : TableSlot.of(attack));
        }
        return new DealState(
                DealPhase.valueOf(node.get("phase").asText()),
                node.hasNonNull("trump") ? Trump.of(Suit.valueOf(node.get("trump").asText())) : null,
                cardList(node.get("deck")),
                players,
                table,
                node.get("roundStarterSeat").asInt(),
                node.get("attackRightSeat").asInt(),
                node.get("defenderSeat").asInt(),
                intList(node.get("passedSeats")),
                intList(node.get("exitOrder")),
                node.get("anyCardBeatenThisRound").asBoolean(),
                node.get("anyPileDiscarded").asBoolean(),
                node.hasNonNull("hangingWindow") ? decodeWindow(node.get("hangingWindow")) : null,
                cardList(node.get("lastAttackCards")),
                node.hasNonNull("pendingHiddenTrump") ? decodePending(node.get("pendingHiddenTrump")) : null,
                node.get("rngSeed").asLong(),
                node.get("diceRolls").asInt());
    }

    private ObjectNode encodePlayer(final PlayerState player) {
        final ObjectNode node = objectMapper.createObjectNode();
        node.put("seatNo", player.seatNo());
        node.set("hand", cardArray(player.hand()));
        player.faceDown().ifPresent(card -> node.put("faceDownCard", CardCodec.encode(card)));
        node.put("inDeal", player.inDeal());
        node.put("navesLevel", player.navesLevel());
        node.set("hungCards", cardArray(player.hungCards()));
        node.put("jokerHangerSeat", player.jokerHangerSeat());
        return node;
    }

    private PlayerState decodePlayer(final JsonNode node) {
        return new PlayerState(
                node.get("seatNo").asInt(),
                cardList(node.get("hand")),
                node.hasNonNull("faceDownCard") ? CardCodec.decode(node.get("faceDownCard").asText()) : null,
                node.get("inDeal").asBoolean(),
                node.get("navesLevel").asInt(),
                cardList(node.get("hungCards")),
                node.get("jokerHangerSeat").asInt());
    }

    private ObjectNode encodeWindow(final HangingWindow window) {
        final ObjectNode node = objectMapper.createObjectNode();
        node.put("victimSeat", window.victimSeat());
        final ArrayNode steps = objectMapper.createArrayNode();
        for (final List<Integer> step : window.steps()) {
            steps.add(intArray(step));
        }
        node.set("steps", steps);
        node.put("stepIndex", window.stepIndex());
        final ArrayNode claims = objectMapper.createArrayNode();
        for (final HangClaim claim : window.claims()) {
            claims.add(objectMapper.createObjectNode()
                    .put("seatNo", claim.seatNo())
                    .put("card", CardCodec.encode(claim.card())));
        }
        node.set("claims", claims);
        node.set("decided", intArray(window.decided()));
        node.put("everyClaimantHangs", window.everyClaimantHangs());
        return node;
    }

    private HangingWindow decodeWindow(final JsonNode node) {
        final List<List<Integer>> steps = new ArrayList<>();
        for (final JsonNode step : node.get("steps")) {
            steps.add(intList(step));
        }
        final List<HangClaim> claims = new ArrayList<>();
        for (final JsonNode claim : node.get("claims")) {
            claims.add(new HangClaim(claim.get("seatNo").asInt(),
                    CardCodec.decode(claim.get("card").asText())));
        }
        return new HangingWindow(node.get("victimSeat").asInt(), steps, node.get("stepIndex").asInt(),
                claims, intList(node.get("decided")), node.get("everyClaimantHangs").asBoolean());
    }

    private PendingHiddenTrump decodePending(final JsonNode node) {
        return new PendingHiddenTrump(CardCodec.decode(node.get("card").asText()),
                node.get("recipientSeat").asInt(), node.get("chooserSeat").asInt());
    }

    private ArrayNode encodeResults(final List<DealOutcome> results) {
        final ArrayNode array = objectMapper.createArrayNode();
        for (final DealOutcome outcome : results) {
            final ObjectNode node = objectMapper.createObjectNode();
            node.put("dealLoserSeat", outcome.dealLoserSeat());
            if (outcome.trumpSuit() != null) {
                node.put("trumpSuit", outcome.trumpSuit().name());
            }
            node.set("lastAttackCards", cardArray(outcome.lastAttackCards()));
            final ArrayNode players = objectMapper.createArrayNode();
            for (final PlayerOutcome player : outcome.players()) {
                final ObjectNode playerNode = objectMapper.createObjectNode()
                        .put("seatNo", player.seatNo())
                        .put("levelBefore", player.levelBefore())
                        .put("levelAfter", player.levelAfter())
                        .put("place", player.place());
                player.degree().ifPresent(degree -> playerNode.put("lossDegree", degree.name()));
                playerNode.set("hungCards", cardArray(player.hungCards()));
                final ArrayNode changes = objectMapper.createArrayNode();
                for (final LevelChange change : player.changes()) {
                    changes.add(objectMapper.createObjectNode()
                            .put("reason", change.reason().name())
                            .put("amount", change.amount()));
                }
                playerNode.set("changes", changes);
                players.add(playerNode);
            }
            node.set("players", players);
            array.add(node);
        }
        return array;
    }

    private List<DealOutcome> decodeResults(final JsonNode array) {
        final List<DealOutcome> results = new ArrayList<>();
        for (final JsonNode node : array) {
            final List<PlayerOutcome> players = new ArrayList<>();
            for (final JsonNode player : node.get("players")) {
                final List<LevelChange> changes = new ArrayList<>();
                for (final JsonNode change : player.get("changes")) {
                    changes.add(new LevelChange(
                            LevelChangeReason.valueOf(change.get("reason").asText()),
                            change.get("amount").asInt()));
                }
                players.add(new PlayerOutcome(player.get("seatNo").asInt(),
                        player.get("levelBefore").asInt(), player.get("levelAfter").asInt(),
                        player.hasNonNull("lossDegree")
                                ? LossDegree.valueOf(player.get("lossDegree").asText()) : null,
                        player.get("place").asInt(), cardList(player.get("hungCards")), changes));
            }
            results.add(new DealOutcome(players, node.get("dealLoserSeat").asInt(),
                    node.hasNonNull("trumpSuit") ? Suit.valueOf(node.get("trumpSuit").asText()) : null,
                    cardList(node.get("lastAttackCards"))));
        }
        return results;
    }

    private ArrayNode cardArray(final List<Card> cards) {
        final ArrayNode array = objectMapper.createArrayNode();
        cards.forEach(card -> array.add(CardCodec.encode(card)));
        return array;
    }

    private List<Card> cardList(final JsonNode array) {
        final List<Card> cards = new ArrayList<>();
        array.forEach(node -> cards.add(CardCodec.decode(node.asText())));
        return cards;
    }

    private ArrayNode intArray(final List<Integer> values) {
        final ArrayNode array = objectMapper.createArrayNode();
        values.forEach(array::add);
        return array;
    }

    private List<Integer> intList(final JsonNode array) {
        final List<Integer> values = new ArrayList<>();
        array.forEach(node -> values.add(node.asInt()));
        return values;
    }
}
