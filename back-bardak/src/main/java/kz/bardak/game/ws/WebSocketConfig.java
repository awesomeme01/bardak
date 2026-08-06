package kz.bardak.game.ws;

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
    private final String[] allowedOrigins;

    public WebSocketConfig(EchoWebSocketHandler echoHandler,
                           @Value("${bardak.ws.allowed-origins}") String[] allowedOrigins) {
        this.echoHandler = echoHandler;
        this.allowedOrigins = allowedOrigins;
    }

    @Override
    public void registerWebSocketHandlers(WebSocketHandlerRegistry registry) {
        // M2: сюда добавится HandshakeInterceptor, проверяющий одноразовый ws-тикет (ADR-005).
        registry.addHandler(echoHandler, "/ws")
                .setAllowedOrigins(allowedOrigins);
    }
}
