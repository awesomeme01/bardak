package kz.bardak.social;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import java.util.Objects;
import java.util.UUID;
import kz.bardak.game.ws.Envelope;
import kz.bardak.push.PushSender;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Component;

/**
 * Доставка приглашения за стол.
 *
 * <p>⭐ Приглашение <b>не хранится</b>. Позвать за стол — это оклик, а не письмо: стол
 * соберётся через минуту, и приглашение, пролежавшее до завтра, ведёт в пустоту. Кто был
 * в сети — услышал сразу, кого не было — не зовут вовсе.
 *
 * <p>Поэтому здесь нет таблицы и нет статусов «принято/отклонено»: друг просто заходит
 * по коду стола, как если бы его позвали голосом.
 */
@Component
public class TableInvites {

    private static final Logger log = LoggerFactory.getLogger(TableInvites.class);

    private final Presence presence;
    private final PushSender push;
    private final ObjectMapper objectMapper;

    public TableInvites(final Presence presence, final PushSender push, final ObjectMapper objectMapper) {
        this.presence = Objects.requireNonNull(presence, "presence");
        this.push = Objects.requireNonNull(push, "push");
        this.objectMapper = Objects.requireNonNull(objectMapper, "objectMapper");
    }

    /**
     * Окликнуть игрока.
     *
     * @return дошло ли по сокету прямо сейчас
     */
    public boolean send(final UUID friendId, final String fromName, final UUID tableId,
                        final String tableName, final String tableCode) {
        final ObjectNode payload = objectMapper.createObjectNode()
                .put("fromName", fromName)
                .put("tableId", tableId.toString())
                .put("tableName", tableName)
                .put("tableCode", tableCode);
        final boolean delivered = presence.send(friendId,
                serialize(Envelope.event("TABLE_INVITE", null, tableId.toString(), payload)));
        if (!delivered && push.isEnabled()) {
            // ⭐ Не в сети — зовём push-ом: это ровно тот случай, ради которого он и заведён.
            push.notifyInvite(friendId, fromName, tableName, tableId.toString());
        }
        log.debug("Приглашение за стол {} игроку {}: доставлено по сокету={}",
                tableId, friendId, delivered);
        return delivered;
    }

    private String serialize(final Envelope envelope) {
        try {
            return objectMapper.writeValueAsString(envelope);
        } catch (final com.fasterxml.jackson.core.JsonProcessingException e) {
            throw new IllegalStateException("Приглашение не сериализуется", e);
        }
    }
}
