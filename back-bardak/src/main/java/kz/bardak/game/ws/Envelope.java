package kz.bardak.game.ws;

import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.databind.JsonNode;

import java.time.Instant;

/**
 * Конверт WebSocket-сообщения, одинаковый в обе стороны.
 * Описан в planning/05-api-contracts.md.
 *
 * @param v        версия протокола; несовпадение → ERROR PROTOCOL_VERSION_UNSUPPORTED
 * @param id       идентификатор сообщения: у команд — для идемпотентности и ack
 * @param type     тип команды (client→server) или события (server→client)
 * @param tableId  стол, к которому относится сообщение; по нему идёт маршрутизация
 * @param seq      только у событий: сквозной номер по матчу
 * @param ts       отметка времени отправителя, epoch millis
 * @param payload  полезная нагрузка, форма зависит от type
 */
@JsonInclude(JsonInclude.Include.NON_NULL)
public record Envelope(
        Integer v,
        String id,
        String type,
        String tableId,
        Integer seq,
        Long ts,
        JsonNode payload
) {

    /** Текущая версия протокола. Ломающее изменение → +1. */
    public static final int PROTOCOL_VERSION = 1;

    /** Событие в ответ на команду: без seq, потому что вне матча нумерации ещё нет. */
    public static Envelope event(String type, String commandId, String tableId, JsonNode payload) {
        return new Envelope(PROTOCOL_VERSION, commandId, type, tableId, null,
                Instant.now().toEpochMilli(), payload);
    }
}
