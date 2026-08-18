package kz.bardak.game.protocol;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.UUID;
import java.util.function.IntFunction;
import kz.bardak.game.rules.Card;
import kz.bardak.game.rules.DealCommand;
import kz.bardak.game.rules.DealEvent;
import kz.bardak.game.rules.PlayerView;
import kz.bardak.game.rules.SeatView;
import kz.bardak.game.rules.TableSlot;

/**
 * Перевод между протоколом и движком.
 *
 * <p>Движок не знает ни про JSON, ни про пользователей — он оперирует местами и картами.
 * Всё сшивание живёт здесь, в одном месте, и потому проверяется одним набором тестов.
 */
public final class GameProtocol {

    private GameProtocol() {
    }

    /**
     * Команда протокола в команду движка.
     *
     * <p>⭐ Защита обязана указывать цель (§2.1): без неё при нескольких картах на столе
     * невозможно зафиксировать, что чем отбито. Поэтому {@code PLAY_CARD} с целью и без —
     * это две разные команды движка, а не одна с необязательным полем.
     */
    public static DealCommand toCommand(final String type, final int seatNo, final Map<String, Object> payload) {
        Objects.requireNonNull(type, "type");
        final Map<String, Object> body = payload == null ? Map.of() : payload;
        return switch (type) {
            case "PLAY_CARD" -> playCard(seatNo, body);
            case "TRANSFER" -> new DealCommand.Transfer(seatNo, card(body, "cardCode"));
            case "PASS" -> new DealCommand.Pass(seatNo);
            case "TAKE" -> new DealCommand.Take(seatNo);
            case "HANG_CARD" -> new DealCommand.HangCard(seatNo, card(body, "cardCode"));
            case "HANG_SKIP" -> new DealCommand.HangSkip(seatNo);
            case "CHOOSE_TRUMP" -> new DealCommand.ChooseTrump(seatNo, suit(body));
            case "REVEAL_FACE_DOWN" -> body.get("targetCardCode") == null
                    ? new DealCommand.RevealFaceDown(seatNo)
                    : new DealCommand.RevealFaceDownToDefend(seatNo, card(body, "targetCardCode"));
            default -> throw new IllegalArgumentException("Неизвестная команда: " + type);
        };
    }

    /** Проекция движка в сообщение `STATE_SYNC`. */
    public static PlayerViewDto toDto(final PlayerView view, final UUID tableId, final int dealNo,
                                      final IntFunction<UUID> userAtSeat,
                                      final IntFunction<String> displayNameAtSeat,
                                      final Integer turnSecondsLeft) {
        final List<PlayerViewDto.SeatStateDto> players = new ArrayList<>();
        for (final SeatView seat : view.seats()) {
            players.add(new PlayerViewDto.SeatStateDto(
                    seat.seatNo(),
                    userAtSeat.apply(seat.seatNo()).toString(),
                    displayNameAtSeat.apply(seat.seatNo()),
                    seat.cardsCount(),
                    seat.hasHiddenCard(),
                    seat.hungCards().stream().map(CardCodec::encode).toList(),
                    seat.navesLevel(),
                    seat.nextRank().map(rank -> rank.code()).orElse(null),
                    seat.nextIsJoker(),
                    seat.passed(),
                    seat.inDeal(),
                    seat.exitPlace(),
                    seat.stepsToJoker()));
        }

        final List<PlayerViewDto.TableSlotDto> table = new ArrayList<>();
        for (final TableSlot slot : view.table()) {
            table.add(new PlayerViewDto.TableSlotDto(CardCodec.encode(slot.attack()),
                    slot.defenceCard().map(CardCodec::encode).orElse(null)));
        }

        return new PlayerViewDto(
                tableId.toString(),
                dealNo,
                view.phase().name(),
                view.trump().map(Enum::name).orElse(null),
                view.trumpCard() == null ? null : CardCodec.encode(view.trumpCard()),
                view.protectedSuit() == null ? null : view.protectedSuit().name(),
                view.deckLeft(),
                view.discardCount(),
                view.myHand().stream().map(CardCodec::encode).toList(),
                view.iHaveHiddenCard(),
                view.mySeat(),
                table,
                players,
                view.roundStarterSeat(),
                view.defenderSeat(),
                view.canAttackSeat(),
                view.hangingVictim().orElse(null),
                turnSecondsLeft,
                view.availableActions().stream().map(GameProtocol::toAction).toList());
    }

    /** Событие движка в payload сообщения. Приватные события фильтрует проекция, не это место. */
    public static Map<String, Object> toEventPayload(final DealEvent event) {
        final Map<String, Object> payload = new LinkedHashMap<>();
        payload.put("seatNo", event.seatNo());
        switch (event) {
            case DealEvent.CardAttacked e -> payload.put("cardCode", CardCodec.encode(e.card()));
            case DealEvent.CardDefended e -> {
                payload.put("cardCode", CardCodec.encode(e.card()));
                payload.put("targetCardCode", CardCodec.encode(e.target()));
            }
            case DealEvent.AttackTransferred e -> {
                payload.put("cardCode", CardCodec.encode(e.card()));
                payload.put("toSeatNo", e.toSeatNo());
            }
            case DealEvent.FaceDownRevealed e -> payload.put("cardCode", CardCodec.encode(e.card()));
            case DealEvent.HiddenTrumpRevealed e -> payload.put("cardCode", CardCodec.encode(e.card()));
            case DealEvent.TrumpChanged e -> payload.put("suit", e.suit().name());
            case DealEvent.TrumpChosen e -> payload.put("suit", e.suit().name());
            case DealEvent.CardHung e -> {
                payload.put("cardCode", CardCodec.encode(e.card()));
                payload.put("victimSeat", e.victimSeat());
            }
            case DealEvent.NavesLevelChanged e -> payload.put("level", e.level());
            case DealEvent.CardsTaken e -> payload.put("count", e.cards().size());
            case DealEvent.RoundBeaten e -> payload.put("count", e.discarded().size());
            case DealEvent.CardsDrawn e -> payload.put("count", e.cards().size());
            case DealEvent.DiceRolled e -> payload.put("participants", e.participants());
            default -> {
                // Остальным событиям хватает места: PASSED, PLAYER_LEFT_DEAL и прочие.
            }
        }
        return payload;
    }

    /** Тип события в имя сообщения протокола: {@code CardAttacked} → {@code CARD_ATTACKED}. */
    public static String eventType(final DealEvent event) {
        final String name = event.getClass().getSimpleName();
        final StringBuilder type = new StringBuilder(name.length() + 4);
        for (int index = 0; index < name.length(); index++) {
            final char symbol = name.charAt(index);
            if (Character.isUpperCase(symbol) && index > 0) {
                type.append('_');
            }
            type.append(Character.toUpperCase(symbol));
        }
        return type.toString();
    }

    private static DealCommand playCard(final int seatNo, final Map<String, Object> payload) {
        final Card card = card(payload, "cardCode");
        final Object target = payload.get("targetCardCode");
        return target == null
                ? new DealCommand.Attack(seatNo, card)
                : new DealCommand.Defend(seatNo, card, CardCodec.decode(target.toString()));
    }

    private static PlayerViewDto.ActionDto toAction(final DealCommand command) {
        final Map<String, Object> payload = new LinkedHashMap<>();
        return switch (command) {
            case DealCommand.Attack c -> {
                payload.put("cardCode", CardCodec.encode(c.card()));
                yield new PlayerViewDto.ActionDto("PLAY_CARD", payload);
            }
            case DealCommand.Defend c -> {
                payload.put("cardCode", CardCodec.encode(c.card()));
                payload.put("targetCardCode", CardCodec.encode(c.target()));
                yield new PlayerViewDto.ActionDto("PLAY_CARD", payload);
            }
            case DealCommand.Transfer c -> {
                payload.put("cardCode", CardCodec.encode(c.card()));
                yield new PlayerViewDto.ActionDto("TRANSFER", payload);
            }
            case DealCommand.HangCard c -> {
                payload.put("cardCode", CardCodec.encode(c.card()));
                yield new PlayerViewDto.ActionDto("HANG_CARD", payload);
            }
            case DealCommand.ChooseTrump c -> {
                payload.put("suit", c.suit().name());
                yield new PlayerViewDto.ActionDto("CHOOSE_TRUMP", payload);
            }
            case DealCommand.RevealFaceDownToDefend c -> {
                payload.put("targetCardCode", CardCodec.encode(c.target()));
                yield new PlayerViewDto.ActionDto("REVEAL_FACE_DOWN", payload);
            }
            case DealCommand.RevealFaceDown c -> new PlayerViewDto.ActionDto("REVEAL_FACE_DOWN", payload);
            case DealCommand.Pass c -> new PlayerViewDto.ActionDto("PASS", payload);
            case DealCommand.Take c -> new PlayerViewDto.ActionDto("TAKE", payload);
            case DealCommand.HangSkip c -> new PlayerViewDto.ActionDto("HANG_SKIP", payload);
        };
    }

    private static Card card(final Map<String, Object> payload, final String field) {
        final Object value = payload.get(field);
        if (value == null) {
            throw new IllegalArgumentException("Не указана карта: " + field);
        }
        return CardCodec.decode(value.toString());
    }

    private static kz.bardak.game.rules.Suit suit(final Map<String, Object> payload) {
        final Object value = payload.get("suit");
        if (value == null) {
            throw new IllegalArgumentException("Не указана масть");
        }
        return kz.bardak.game.rules.Suit.valueOf(value.toString().toUpperCase(java.util.Locale.ROOT));
    }
}
