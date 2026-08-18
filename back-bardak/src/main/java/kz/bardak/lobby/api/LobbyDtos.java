package kz.bardak.lobby.api;

import jakarta.validation.constraints.Max;
import jakarta.validation.constraints.Min;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.Size;
import java.util.List;
import java.util.Map;
import java.util.UUID;

/** Формы лобби (`05-api-contracts.md`). */
public final class LobbyDtos {

    private LobbyDtos() {
    }

    /**
     * @param rulesConfig игровые числа стола; пусто — берутся значения по умолчанию (ADR-016)
     */
    public record CreateTableRequest(
            @NotBlank @Size(min = 2, max = 64) String name,
            @Min(2) @Max(5) int maxPlayers,
            UUID cardSetId,
            UUID themeId,
            Map<String, Object> rulesConfig,
            boolean isPrivate) {
    }

    public record SeatView(int seatNo, String userId, String displayName, boolean ready, boolean online) {
    }

    public record TableView(
            String id,
            String code,
            String name,
            String hostUserId,
            int maxPlayers,
            String status,
            String cardSetId,
            String themeId,
            boolean isPrivate,
            List<SeatView> seats) {
    }

    /**
     * Где игрок сидит сейчас.
     *
     * @param table     стол или {@code null}, если он нигде не сидит
     * @param inMatch   за столом идёт матч — возвращаться нужно немедленно
     * @param mySeatNo  его место за этим столом
     */
    public record CurrentTableView(TableView table, boolean inMatch, Integer mySeatNo) {
    }

    /**
     * Стол глазами того, кто пришёл по ссылке и ещё не вошёл в игру.
     *
     * <p>⭐ Отдаётся <b>без токена</b>: по ссылке приходит и тот, у кого учётки нет вовсе,
     * и до регистрации он должен видеть, куда его зовут. Иначе приглашение выглядит как
     * пустая форма входа, и человек уходит, не поняв, зачем ему регистрироваться.
     *
     * <p>⚠️ Здесь намеренно НЕТ имён игроков и идентификаторов — только сам стол и сколько
     * мест занято. Код стола короткий, его пересылают в переписке, и всё, что попадёт
     * в этот ответ, станет доступно любому, кто код увидел.
     *
     * @param joinable  за стол ещё можно сесть: не идёт матч и есть свободное место
     */
    public record TableInviteView(String code, String name, int maxPlayers, int seatsTaken,
                                  boolean isPrivate, boolean joinable) {
    }

    public record CardSetView(String id, String code, String name, String description,
                              String version, String previewUrl, boolean isDefault) {
    }

    /** Манифест: код карты → URL. Больше клиенту про набор знать нечего (ADR-009). */
    public record CardSetManifest(String id, String code, String version, Map<String, String> cards) {
    }

    public record TableThemeView(String id, String code, String name, String feltColor,
                                 String defaultBackCode, boolean isDefault) {
    }
}
