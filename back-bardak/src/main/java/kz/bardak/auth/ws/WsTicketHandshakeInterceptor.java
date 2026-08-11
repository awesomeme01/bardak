package kz.bardak.auth.ws;

import java.util.Map;
import java.util.Objects;
import java.util.UUID;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.http.server.ServerHttpRequest;
import org.springframework.http.server.ServerHttpResponse;
import org.springframework.http.server.ServletServerHttpRequest;
import org.springframework.stereotype.Component;
import org.springframework.web.socket.WebSocketHandler;
import org.springframework.web.socket.server.HandshakeInterceptor;

/**
 * Пускает в сокет только по одноразовому тикету (ADR-005).
 *
 * <p>⭐ Проверка происходит <b>до</b> установления соединения: неавторизованный клиент
 * получает 401 на рукопожатии и не занимает ни сессию, ни память.
 *
 * <p>Тикет гасится здесь же — повторно тем же тикетом не подключиться.
 */
@Component
public class WsTicketHandshakeInterceptor implements HandshakeInterceptor {

    /** Ключ, под которым игрок лежит в атрибутах сессии; дальше его читает хендлер. */
    public static final String USER_ID_ATTRIBUTE = "userId";

    private static final Logger log = LoggerFactory.getLogger(WsTicketHandshakeInterceptor.class);
    private static final String TICKET_PARAMETER = "ticket";

    private final WsTicketService tickets;

    public WsTicketHandshakeInterceptor(final WsTicketService tickets) {
        this.tickets = Objects.requireNonNull(tickets, "tickets");
    }

    @Override
    public boolean beforeHandshake(final ServerHttpRequest request, final ServerHttpResponse response,
                                   final WebSocketHandler handler, final Map<String, Object> attributes) {
        final UUID userId = tickets.consume(ticketFrom(request)).orElse(null);
        if (userId == null) {
            response.setStatusCode(HttpStatus.UNAUTHORIZED);
            log.debug("WS-рукопожатие отклонено: тикет отсутствует, просрочен или уже погашен");
            return false;
        }
        attributes.put(USER_ID_ATTRIBUTE, userId);
        return true;
    }

    @Override
    public void afterHandshake(final ServerHttpRequest request, final ServerHttpResponse response,
                               final WebSocketHandler handler, final Exception exception) {
        // Ничего: всё, что нужно, уже положено в атрибуты сессии.
    }

    private String ticketFrom(final ServerHttpRequest request) {
        if (request instanceof ServletServerHttpRequest servletRequest) {
            return servletRequest.getServletRequest().getParameter(TICKET_PARAMETER);
        }
        return null;
    }
}
