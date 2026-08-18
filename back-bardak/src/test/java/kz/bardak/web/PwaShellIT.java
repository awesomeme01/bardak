package kz.bardak.web;

import static org.assertj.core.api.Assertions.assertThat;

import kz.bardak.TestPostgres;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.web.client.TestRestTemplate;
import org.springframework.boot.testcontainers.service.connection.ServiceConnection;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.testcontainers.containers.PostgreSQLContainer;

/**
 * Оболочка приложения отдаётся без токена.
 *
 * <p>⭐ Иначе браузер просто не считает приложение устанавливаемым: манифест, иконки
 * и воркер он читает сам, без заголовка авторизации, и на 401 молча отказывается
 * предлагать установку. Проверка дешёвая, а поломка — незаметная.
 */
@Tag("integration")
@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT)
class PwaShellIT {

    @ServiceConnection
    static final PostgreSQLContainer<?> POSTGRES = TestPostgres.INSTANCE;

    @Autowired
    private TestRestTemplate rest;

    @DisplayName("Should serve the manifest without a token When the browser asks for it")
    @Test
    void shouldServeTheManifestWithoutATokenWhenTheBrowserAsksForIt() {
        final ResponseEntity<String> response = rest.getForEntity("/manifest.webmanifest", String.class);

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.OK);
        assertThat(response.getBody()).contains("\"start_url\"").contains("icon-512.png");
    }

    @DisplayName("Should serve the service worker without a token When the browser registers it")
    @Test
    void shouldServeTheServiceWorkerWithoutATokenWhenTheBrowserRegistersIt() {
        final ResponseEntity<String> response = rest.getForEntity("/sw.js", String.class);

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.OK);
        // ⚠️ Воркер обязан обходить API стороной: ответ из кэша на «начать матч» — ложь,
        // которую клиент не отличит от правды.
        assertThat(response.getBody()).contains("'/api'");
    }

    @DisplayName("Should serve the icon without a token When the manifest points at it")
    @Test
    void shouldServeTheIconWithoutATokenWhenTheManifestPointsAtIt() {
        assertThat(rest.getForEntity("/icons/icon-192.png", byte[].class).getStatusCode())
                .isEqualTo(HttpStatus.OK);
    }

    @DisplayName("Should still demand a token When the history is asked for")
    @Test
    void shouldStillDemandATokenWhenTheHistoryIsAskedFor() {
        // Граница не поехала: открыли оболочку, а не данные.
        assertThat(rest.getForEntity("/api/matches", String.class).getStatusCode())
                .isEqualTo(HttpStatus.UNAUTHORIZED);
    }
}
