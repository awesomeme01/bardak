package kz.bardak.rating;

import java.util.List;
import java.util.Set;
import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Настройки рейтинга.
 *
 * @param seasonAdmins логины тех, кто вправе закрыть сезон. ⭐ Ролей в системе нет и заводить
 *                     их ради одной кнопки незачем: игра для узкого круга, а сезон
 *                     закрывается вручную и редко (ADR-037). Появятся другие
 *                     административные действия — появится и роль
 */
@ConfigurationProperties(prefix = "bardak.rating")
public record RatingProperties(List<String> seasonAdmins) {

    public RatingProperties {
        seasonAdmins = seasonAdmins == null ? List.of() : List.copyOf(seasonAdmins);
    }

    public boolean isSeasonAdmin(final String username) {
        return Set.copyOf(seasonAdmins).contains(username);
    }
}
