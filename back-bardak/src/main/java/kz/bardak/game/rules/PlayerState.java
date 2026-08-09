package kz.bardak.game.rules;

import java.util.ArrayList;
import java.util.List;
import java.util.Objects;
import java.util.Optional;

/**
 * Состояние игрока внутри раздачи.
 *
 * <p>⭐ У руки два разных счёта, и их легко перепутать (§1.8): {@link #handSize()} — для
 * добора, скрытая карта в него не входит; {@link #defendableCards(boolean)} — для лимита
 * атаки, и там она считается, но только при пустой колоде (ADR-034).
 *
 * <p>⭐ Навесы распадаются на две части с разным временем жизни (ADR-018): {@code navesLevel}
 * живёт весь матч и переживает перераздачу, {@code hungCards} очищается вместе с раздачей —
 * карты возвращаются в колоду. Путать их нельзя.
 *
 * @param seatNo       место за столом, фиксировано на весь матч
 * @param hand         открытые для владельца карты
 * @param faceDownCard скрытая карта; {@code null}, если уже вскрыта. Её не видит никто,
 *                     включая владельца (ADR-026)
 * @param inDeal       игрок ещё в раздаче — не вышел и не выбыл
 * @param navesLevel   уровень по шкале навесов; {@link NavesScale#NO_NAVES} — навесов не было
 * @param hungCards    карты, навешенные ему в этой раздаче; видны всем (§2.3)
 */
public record PlayerState(
        int seatNo,
        List<Card> hand,
        Card faceDownCard,
        boolean inDeal,
        int navesLevel,
        List<Card> hungCards) {

    public PlayerState {
        if (seatNo < 0) {
            throw new IllegalArgumentException("Место за столом не может быть отрицательным: " + seatNo);
        }
        hand = List.copyOf(Objects.requireNonNull(hand, "hand"));
        hungCards = List.copyOf(Objects.requireNonNull(hungCards, "hungCards"));
    }

    public static PlayerState of(final int seatNo, final List<Card> hand, final Card faceDownCard) {
        return new PlayerState(seatNo, hand, faceDownCard, true, NavesScale.NO_NAVES, List.of());
    }

    /** Счёт для добора: скрытая карта в него не входит (§1.8). */
    public int handSize() {
        return hand.size();
    }

    public boolean hasFaceDownCard() {
        return faceDownCard != null;
    }

    public Optional<Card> faceDown() {
        return Optional.ofNullable(faceDownCard);
    }

    /**
     * Счёт для лимита атаки: сколько карт игрок физически способен положить в защиту.
     *
     * <p>⭐ Скрытая карта входит сюда, <b>только когда колода пуста</b> (ADR-034). Пока в
     * колоде есть карты, атака не вправе вынудить её вскрыть.
     */
    public int defendableCards(final boolean deckEmpty) {
        return handSize() + (deckEmpty && hasFaceDownCard() ? 1 : 0);
    }

    public boolean holdsInHand(final Card card) {
        Objects.requireNonNull(card, "card");
        return hand.contains(card);
    }

    /**
     * Скрытая карта играется, только когда колода пуста и обычных карт не осталось (§1.8).
     * Открытие необратимо, но это уже переход состояния, а не проверка.
     */
    public boolean canPlayFaceDown(final boolean deckEmpty) {
        return deckEmpty && hasFaceDownCard() && hand.isEmpty();
    }

    public PlayerState withHand(final List<Card> value) {
        return new PlayerState(seatNo, value, faceDownCard, inDeal, navesLevel, hungCards);
    }

    /** Карта сыграна из руки. */
    public PlayerState withoutCard(final Card card) {
        final List<Card> remaining = new ArrayList<>(hand);
        remaining.remove(card);
        return withHand(List.copyOf(remaining));
    }

    /** Скрытая карта вскрыта — переход односторонний (§1.8). */
    public PlayerState withFaceDownRevealed() {
        return new PlayerState(seatNo, hand, null, inDeal, navesLevel, hungCards);
    }

    public PlayerState leftDeal() {
        return new PlayerState(seatNo, hand, faceDownCard, false, navesLevel, hungCards);
    }

    public PlayerState withNavesLevel(final int value) {
        return new PlayerState(seatNo, hand, faceDownCard, inDeal, value, hungCards);
    }

    /** Карта уходит из чужой руки в этот слот и выбывает из игры до конца раздачи (§2.3). */
    public PlayerState withHungCard(final Card card) {
        final List<Card> slot = new ArrayList<>(hungCards);
        slot.add(Objects.requireNonNull(card, "card"));
        return new PlayerState(seatNo, hand, faceDownCard, inDeal, navesLevel, List.copyOf(slot));
    }
}
