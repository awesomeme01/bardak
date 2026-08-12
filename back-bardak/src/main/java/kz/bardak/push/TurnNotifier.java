package kz.bardak.push;

import java.time.Clock;
import java.time.Instant;
import java.util.Map;
import java.util.Objects;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Component;

/**
 * Кому и когда звонить «твой ход».
 *
 * <p>⭐ Уведомление уходит <b>только тому, кого нет за столом</b>. Игроку с открытой вкладкой
 * звонить не нужно — он и так видит ход; звонок в этом случае не помогает, а раздражает
 * и быстро приводит к тому, что уведомления отключают целиком.
 *
 * <p>⭐ Второе ограничение — тишина после звонка. Ход может вернуться к игроку через
 * несколько секунд (отбился, подкинули, снова его очередь), и без паузы партия
 * превратилась бы в очередь звонков. Один звонок за окно тишины на игрока.
 */
@Component
public class TurnNotifier {

    private static final Logger log = LoggerFactory.getLogger(TurnNotifier.class);

    private final PushSender sender;
    private final PushProperties properties;
    private final Clock clock;

    /** Когда последний раз звонили. Ключ — игрок: окно тишины персональное. */
    private final Map<UUID, Instant> lastNotified = new ConcurrentHashMap<>();

    public TurnNotifier(final PushSender sender, final PushProperties properties, final Clock clock) {
        this.sender = Objects.requireNonNull(sender, "sender");
        this.properties = Objects.requireNonNull(properties, "properties");
        this.clock = Objects.requireNonNull(clock, "clock");
    }

    /**
     * Ход перешёл к игроку.
     *
     * @param present есть ли игрок за столом прямо сейчас
     */
    public void turnOf(final UUID userId, final String tableName, final UUID tableId,
                       final boolean present) {
        if (!sender.isEnabled() || present) {
            return;
        }
        final Instant now = clock.instant();
        final Instant last = lastNotified.get(userId);
        if (last != null && last.plus(properties.quietFor()).isAfter(now)) {
            return;
        }
        lastNotified.put(userId, now);
        log.debug("Зову игрока {} к столу {}", userId, tableId);
        sender.notifyTurn(userId, tableName, tableId.toString());
    }

    /**
     * Игрок пропал, и матч встал из-за него на паузу.
     *
     * <p>Окно тишины здесь не применяется: пауза случается редко, а цена молчания —
     * отменённый матч у всех за столом.
     */
    public void pausedFor(final UUID userId, final String tableName, final UUID tableId,
                          final long secondsLeft) {
        if (!sender.isEnabled()) {
            return;
        }
        lastNotified.put(userId, clock.instant());
        log.debug("Зову игрока {} обратно к столу {}", userId, tableId);
        sender.notifyPaused(userId, tableName, tableId.toString(), secondsLeft);
    }

    /** Игрок вернулся за стол: следующий его ход снова достоин звонка. */
    public void present(final UUID userId) {
        lastNotified.remove(userId);
    }
}
