package kz.bardak.game.ws;

import kz.bardak.auth.ws.WsTicketHandshakeInterceptor;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Configuration;
import org.springframework.web.socket.config.annotation.EnableWebSocket;
import org.springframework.web.socket.config.annotation.WebSocketConfigurer;
import org.springframework.web.socket.config.annotation.WebSocketHandlerRegistry;

/**
 * Регистрация WS-эндпоинта. Сырой WebSocket + JSON, без STOMP — ADR-002:
 * нам нужны персональные проекции состояния (каждому игроку своё сообщение),
 * а модель STOMP «топик → все подписчики получают одно и то же» работает против этого.
 */
@Configuration
@EnableWebSocket
public class WebSocketConfig implements WebSocketConfigurer {

    private final EchoWebSocketHandler echoHandler;
    private final WsTicketHandshakeInterceptor ticketInterceptor;
    private final String[] allowedOrigins;

    public WebSocketConfig(EchoWebSocketHandler echoHandler,
                           WsTicketHandshakeInterceptor ticketInterceptor,
                           @Value("${bardak.ws.allowed-origins}") String[] allowedOrigins) {
        this.echoHandler = echoHandler;
        this.ticketInterceptor = ticketInterceptor;
        this.allowedOrigins = allowedOrigins;
    }

    @Override
    public void registerWebSocketHandlers(WebSocketHandlerRegistry registry) {
        // Одноразовый тикет проверяется до установления соединения (ADR-005):
        // неавторизованный клиент получает 401 на рукопожатии и сессию не занимает.
        registry.addHandler(echoHandler, "/ws")
                .addInterceptors(ticketInterceptor)
                .setAllowedOrigins(allowedOrigins);
    }
}
