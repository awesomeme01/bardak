package kz.bardak.web;

import static org.assertj.core.api.Assertions.assertThat;

import com.fasterxml.jackson.databind.JsonNode;
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
 * Неизвестный адрес под {@code /api}.
 *
 * <p>⚠️ Проверка появилась после находки: снесённая ручка отвечала <b>500</b>, а не 404.
 * Spring бросает {@code NoResourceFoundException}, и она проваливалась в обработчик
 * «необработанной ошибки» — клиент не мог отличить «такого адреса нет» от «сервер
 * сломался», а лог наполнялся стеками там, где ничего не ломалось.
 */
@Tag("integration")
@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT)
class UnknownPathIT {

    @ServiceConnection
    static final PostgreSQLContainer<?> POSTGRES = TestPostgres.INSTANCE;

    @Autowired
    private TestRestTemplate rest;

    @DisplayName("Should answer not found When the path does not exist")
    @Test
    void shouldAnswerNotFoundWhenThePathDoesNotExist() {
        final ResponseEntity<JsonNode> response =
                rest.getForEntity("/api/tables/invite/AAAAAA/nope", JsonNode.class);

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.NOT_FOUND);
        assertThat(response.getBody().get("code").asText()).isEqualTo("NOT_FOUND");
        // Конверт ошибки — тот же, что у всех: traceId на месте, разбираться есть по чему.
        assertThat(response.getBody().get("traceId").asText()).isNotBlank();
    }
}
