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
