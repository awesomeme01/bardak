package kz.bardak.game.ws;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Component;
import org.springframework.web.socket.CloseStatus;
import org.springframework.web.socket.TextMessage;
import org.springframework.web.socket.WebSocketSession;
import org.springframework.web.socket.handler.ConcurrentWebSocketSessionDecorator;
import org.springframework.web.socket.handler.TextWebSocketHandler;

import java.io.IOException;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

/**
 * M1: эхо-обработчик. Никакой игровой логики — только проверка того, что конверт
 * протокола ходит в обе стороны и соединение живёт.
 *
 * <p>Что здесь уже настоящее и останется дальше:
 * <ul>
 *   <li>разбор и валидация конверта {@link Envelope}, включая версию протокола;</li>
 *   <li>PING → PONG: прикладной heartbeat, потому что TCP-таймауты измеряются
 *       минутами, а ход в игре длится 30 секунд;</li>
 *   <li>обёртка сессии в {@link ConcurrentWebSocketSessionDecorator} — WebSocketSession
 *       не потокобезопасна, а писать в неё будут поток стола и планировщик таймеров.</li>
 * </ul>
 *
 * <p>Чего здесь ещё нет: аутентификации (M2, одноразовый ws-тикет), маршрутизации
 * к столу и очереди команд (M3), игровых команд (M4).
 */
@Component
public class EchoWebSocketHandler extends TextWebSocketHandler {

    private static final Logger log = LoggerFactory.getLogger(EchoWebSocketHandler.class);

    /** Лимит буфера на медленного клиента: не даём одному соединению есть память. */
    private static final int SEND_BUFFER_LIMIT = 512 * 1024;
    private static final int SEND_TIME_LIMIT_MS = 10_000;

    private final ObjectMapper objectMapper;
    private final Map<String, WebSocketSession> sessions = new ConcurrentHashMap<>();

    public EchoWebSocketHandler(ObjectMapper objectMapper) {
        this.objectMapper = objectMapper;
    }

    @Override
    public void afterConnectionEstablished(WebSocketSession session) {
        WebSocketSession concurrent = new ConcurrentWebSocketSessionDecorator(
                session, SEND_TIME_LIMIT_MS, SEND_BUFFER_LIMIT);
        sessions.put(session.getId(), concurrent);
        log.info("WS подключён: id={}, всего сессий={}", session.getId(), sessions.size());

        ObjectNode payload = objectMapper.createObjectNode()
                .put("sessionId", session.getId())
                .put("protocolVersion", Envelope.PROTOCOL_VERSION);
        send(concurrent, Envelope.event("CONNECTED", null, null, payload));
    }

    @Override
    protected void handleTextMessage(WebSocketSession session, TextMessage message) {
        WebSocketSession out = sessions.getOrDefault(session.getId(), session);

        Envelope incoming;
        try {
            incoming = objectMapper.readValue(message.getPayload(), Envelope.class);
        } catch (IOException e) {
            send(out, error(null, "BAD_ENVELOPE", "Сообщение не разобрано как конверт протокола"));
            return;
        }

        if (incoming.v() == null || incoming.v() != Envelope.PROTOCOL_VERSION) {
            send(out, error(incoming.id(), "PROTOCOL_VERSION_UNSUPPORTED",
                    "Поддерживается версия протокола " + Envelope.PROTOCOL_VERSION));
            return;
        }
        if (incoming.type() == null || incoming.type().isBlank()) {
            send(out, error(incoming.id(), "TYPE_REQUIRED", "Не указан тип сообщения"));
            return;
        }

        if ("PING".equals(incoming.type())) {
            send(out, Envelope.event("PONG", incoming.id(), incoming.tableId(), null));
            return;
        }

        // M1: всё остальное возвращаем как есть — это и есть эхо.
        ObjectNode payload = objectMapper.createObjectNode();
        payload.put("echoOf", incoming.type());
        payload.set("payload", incoming.payload());
        send(out, Envelope.event("ECHO", incoming.id(), incoming.tableId(), payload));
    }

    @Override
    public void afterConnectionClosed(WebSocketSession session, CloseStatus status) {
        sessions.remove(session.getId());
        log.info("WS отключён: id={}, статус={}, осталось сессий={}",
                session.getId(), status.getCode(), sessions.size());
    }

    @Override
    public void handleTransportError(WebSocketSession session, Throwable exception) {
        log.warn("Ошибка транспорта WS: id={}, {}", session.getId(), exception.toString());
    }

    private Envelope error(String commandId, String code, String message) {
        ObjectNode payload = objectMapper.createObjectNode()
                .put("code", code)
                .put("message", message);
        return Envelope.event("ERROR", commandId, null, payload);
    }

    private void send(WebSocketSession session, Envelope envelope) {
        try {
            session.sendMessage(new TextMessage(objectMapper.writeValueAsString(envelope)));
        } catch (IOException e) {
            log.warn("Не удалось отправить {} в сессию {}: {}",
                    envelope.type(), session.getId(), e.getMessage());
        }
    }
}
