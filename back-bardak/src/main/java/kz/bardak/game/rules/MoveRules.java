package kz.bardak.game.rules;

import java.util.Objects;

/**
 * Легальность хода: чистые проверки над снимком раздачи, без состояния и без побочных
 * эффектов. Здесь живёт вся спецификация каркаса — §1.5, §1.8, §2.1, §2.2, §1.1.1.
 *
 * <p>Проверки намеренно отделены от переходов: сервер обязан валидировать каждый ход
 * (ADR-003), а отклонённый ход не меняет состояние вообще.
 */
public final class MoveRules {

    private final RulesConfig config;

    public MoveRules(final RulesConfig config) {
        this.config = Objects.requireNonNull(config, "config");
    }

    /**
     * Можно ли положить карту в атаку — первой картой раунда или подкидом (§1.5, §2.1).
     */
    public MoveVerdict canAttack(final DealState state, final int seatNo, final Card card) {
        Objects.requireNonNull(state, "state");
        Objects.requireNonNull(card, "card");
        if (state.attackRightSeat() != seatNo || state.hasPassed(seatNo)) {
            return MoveVerdict.rejected(RejectionReason.NOT_YOUR_TURN);
        }
        final MoveVerdict holding = canPlayFromHand(state, seatNo, card);
        if (!holding.isAllowed()) {
            return holding;
        }
        if (state.attackCardCount() >= config.attackLimit(state.anyPileDiscarded())) {
            return MoveVerdict.rejected(RejectionReason.ATTACK_LIMIT_REACHED);
        }
        if (!state.table().isEmpty() && !state.hasRankOnTable(card)) {
            return MoveVerdict.rejected(RejectionReason.RANK_NOT_ON_TABLE);
        }
        if (state.unbeatenCount() + 1 > state.defender().defendableCards(state.isDeckEmpty())) {
            return MoveVerdict.rejected(RejectionReason.DEFENDER_HAS_TOO_FEW_CARDS);
        }
        return MoveVerdict.allowed();
    }

    /**
     * Можно ли отбить конкретную атакующую карту (§1.1.1, §2.1). Цель обязательна: при
     * нескольких картах на столе иначе не зафиксировать, что чем отбито.
     */
    public MoveVerdict canDefend(final DealState state, final int seatNo, final Card card, final Card target) {
        Objects.requireNonNull(state, "state");
        Objects.requireNonNull(card, "card");
        Objects.requireNonNull(target, "target");
        if (state.defenderSeat() != seatNo) {
            return MoveVerdict.rejected(RejectionReason.NOT_YOUR_TURN);
        }
        final MoveVerdict holding = canPlayFromHand(state, seatNo, card);
        if (!holding.isAllowed()) {
            return holding;
        }
        final TableSlot slot = state.table().stream()
                .filter(candidate -> candidate.attack().equals(target))
                .findFirst()
                .orElse(null);
        if (slot == null) {
            return MoveVerdict.rejected(RejectionReason.TARGET_NOT_ON_TABLE);
        }
        if (slot.isBeaten()) {
            return MoveVerdict.rejected(RejectionReason.TARGET_ALREADY_BEATEN);
        }
        if (!state.trump().beats(card, target)) {
            return MoveVerdict.rejected(RejectionReason.CARD_DOES_NOT_BEAT);
        }
        return MoveVerdict.allowed();
    }

    /**
     * Можно ли перевести атаку дальше по кругу (§2.2, ADR-031). Перевод жив, только пока
     * не отбита ни одна карта, — и потому вся переводимая атака всегда одноранговая.
     */
    public MoveVerdict canTransfer(final DealState state, final int seatNo, final Card card) {
        Objects.requireNonNull(state, "state");
        Objects.requireNonNull(card, "card");
        if (!config.transfersEnabled()) {
            return MoveVerdict.rejected(RejectionReason.TRANSFERS_DISABLED);
        }
        if (state.defenderSeat() != seatNo) {
            return MoveVerdict.rejected(RejectionReason.NOT_YOUR_TURN);
        }
        final MoveVerdict holding = canPlayFromHand(state, seatNo, card);
        if (!holding.isAllowed()) {
            return holding;
        }
        if (state.table().isEmpty()) {
            return MoveVerdict.rejected(RejectionReason.TRANSFER_RANK_MISMATCH);
        }
        if (state.anyCardBeatenThisRound()) {
            return MoveVerdict.rejected(RejectionReason.TRANSFER_AFTER_FIRST_BEAT);
        }
        if (!state.table().get(0).attack().sameRankAs(card)) {
            return MoveVerdict.rejected(RejectionReason.TRANSFER_RANK_MISMATCH);
        }
        return verdictOnReceiver(state, seatNo);
    }

    /**
     * Принимающему должно хватать карт отбить выросшую атаку — иначе перевод поставил бы
     * его в заведомо безвыходное положение (§2.2).
     */
    private MoveVerdict verdictOnReceiver(final DealState state, final int seatNo) {
        final PlayerState receiver = state.playerAt(state.nextActiveSeatAfter(seatNo));
        final int attackAfterTransfer = state.attackCardCount() + 1;
        if (receiver.defendableCards(state.isDeckEmpty()) < attackAfterTransfer) {
            return MoveVerdict.rejected(RejectionReason.NEXT_PLAYER_HAS_TOO_FEW_CARDS);
        }
        return MoveVerdict.allowed();
    }

    /**
     * Держит ли игрок эту карту и вправе ли ею играть. Скрытая карта — особый случай:
     * она играется, только когда колода пуста и обычных карт не осталось (§1.8).
     */
    private MoveVerdict canPlayFromHand(final DealState state, final int seatNo, final Card card) {
        final PlayerState player = state.playerAt(seatNo);
        if (player.holdsInHand(card)) {
            return MoveVerdict.allowed();
        }
        if (player.faceDown().filter(card::equals).isEmpty()) {
            return MoveVerdict.rejected(RejectionReason.CARD_NOT_IN_HAND);
        }
        if (!player.canPlayFaceDown(state.isDeckEmpty())) {
            return MoveVerdict.rejected(RejectionReason.FACE_DOWN_CARD_NOT_PLAYABLE);
        }
        return MoveVerdict.allowed();
    }
}
