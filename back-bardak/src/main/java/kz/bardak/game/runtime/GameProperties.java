package kz.bardak.game.runtime;

import java.time.Duration;
import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Времена стола (§5.1, §5.2).
 *
 * <p>Живут в конфиге приложения, а не в коде: значения из `03-domain-rules.md` §1.6
 * и меняются под стенд. В тестах ставятся секундами, чтобы не ждать минуту.
 *
 * @param turnTimeout       ход игрока; по истечении сервер делает самое безобидное действие
 * @param autoMoveOnTimeout ходить ли за молчащего вообще
 * @param disconnectGrace   сколько ждать вернувшегося, прежде чем отменить матч
 */
@ConfigurationProperties(prefix = "bardak.game")
public record GameProperties(Duration turnTimeout, boolean autoMoveOnTimeout, Duration disconnectGrace) {

    public GameProperties {
        turnTimeout = turnTimeout == null ? Duration.ofSeconds(30) : turnTimeout;
        disconnectGrace = disconnectGrace == null ? Duration.ofSeconds(60) : disconnectGrace;
    }

    /**
     * ⭐ По умолчанию сервер за игрока не ходит: ход просто ждёт своего хозяина.
     *
     * <p>Игра для своих, и отобранный ход раздражает сильнее, чем сосед, отошедший за чаем.
     * Часы включаются настройкой — там, где за столом сидят чужие и ждать некого.
     */
    public static GameProperties defaults() {
        return new GameProperties(Duration.ofSeconds(30), false, Duration.ofSeconds(60));
    }
}
