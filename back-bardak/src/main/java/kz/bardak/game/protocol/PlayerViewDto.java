package kz.bardak.game.protocol;

import java.util.List;
import java.util.Map;

/**
 * `STATE_SYNC` — персональная проекция состояния для одного игрока
 * (`05-api-contracts.md`).
 *
 * <p>⭐ Строится из {@link kz.bardak.game.rules.PlayerView}, в котором чужих карт нет
 * физически. Здесь только перевод в коды карт и места за столом — никакой фильтрации:
 * фильтровать было бы поздно и опасно.
 *
 * @param myHand           своя рука; у соседей — только {@code cardsCount}
 * @param iHaveHiddenCard  своя скрытая карта: только факт, даже владелец её не видит (§1.8)
 * @param trumpCard        козырная карта, лежащая под колодой, — видна всем (§1.9);
 *                         {@code null}, когда её уже забрали или козырь назван костью
 * @param availableActions что именно можно сделать сейчас; фронт правил не знает (ADR-003)
 * @param turnSecondsLeft  сколько секунд осталось у того, кто на часах (§5.1); {@code null},
 *                         если ход никого не ждёт
 */
public record PlayerViewDto(
        String tableId,
        int dealNo,
        String phase,
        String trumpSuit,
        String trumpCard,
        String protectedSuit,
        int deckLeft,
        int discardCount,
        List<String> myHand,
        boolean iHaveHiddenCard,
        int mySeat,
        List<TableSlotDto> table,
        List<SeatStateDto> players,
        int attackerSeat,
        int defenderSeat,
        int canAttackSeat,
        Integer hangingVictimSeat,
        Integer turnSecondsLeft,
        List<ActionDto> availableActions) {

    public record TableSlotDto(String attack, String defend) {
    }

    /**
     * @param navesLevel    уровень по шкале; переносится между раздачами
     * @param nextNavesRank что можно навесить следующим — считает сервер, чтобы фронт
     *                      не воспроизводил шкалу
     * @param exitPlace     каким по счёту вышел из раздачи; {@code null} — ещё играет
     * @param stepsToJoker  сколько навесов осталось до джокера
     */
    public record SeatStateDto(int seatNo, String userId, String displayName, int cardsCount,
                               boolean hasHiddenCard, List<String> hung, int navesLevel,
                               String nextNavesRank, boolean nextIsJoker, boolean passed,
                               boolean inDeal, Integer exitPlace, int stepsToJoker) {
    }

    /** Разрешённое действие. Форма совпадает с командой, которую можно прислать обратно. */
    public record ActionDto(String type, Map<String, Object> payload) {
    }
}
