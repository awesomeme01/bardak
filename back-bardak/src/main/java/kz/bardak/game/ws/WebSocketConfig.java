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
    private final String[] allowedOriginPatterns;

    public WebSocketConfig(EchoWebSocketHandler echoHandler,
                           WsTicketHandshakeInterceptor ticketInterceptor,
                           @Value("${bardak.ws.allowed-origins}") String[] allowedOrigins,
                           @Value("${bardak.ws.allowed-origin-patterns:}") String[] allowedOriginPatterns) {
        this.echoHandler = echoHandler;
        this.ticketInterceptor = ticketInterceptor;
        this.allowedOrigins = allowedOrigins;
        this.allowedOriginPatterns = allowedOriginPatterns;
    }

    @Override
    public void registerWebSocketHandlers(WebSocketHandlerRegistry registry) {
        // Одноразовый тикет проверяется до установления соединения (ADR-005):
        // неавторизованный клиент получает 401 на рукопожатии и сессию не занимает.
        final var handler = registry.addHandler(echoHandler, "/ws")
                .addInterceptors(ticketInterceptor)
                .setAllowedOrigins(allowedOrigins);
        // ⭐ Шаблоны нужны для игры по локальной сети: адрес машины меняется от роутера
        // к роутеру, и перечислять его руками пришлось бы каждый раз заново. Шаблон
        // ограничен частными диапазонами — снаружи по такому адресу не придут.
        final String[] patterns = java.util.Arrays.stream(allowedOriginPatterns)
                .filter(pattern -> !pattern.isBlank())
                .toArray(String[]::new);
        if (patterns.length > 0) {
            handler.setAllowedOriginPatterns(patterns);
        }
    }
}
