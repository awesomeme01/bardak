package kz.bardak.common.web;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.time.Instant;
import java.util.Map;

/**
 * Проверка живости связки «приложение → база». M1: единственный REST-эндпоинт.
 *
 * <p>Отдельно от /actuator/health намеренно: actuator — для мониторинга,
 * а этот эндпоинт часть публичного API и отвечает в формате, описанном
 * в planning/05-api-contracts.md.
 */
@RestController
@RequestMapping("/api")
public class HealthController {

    private static final Logger log = LoggerFactory.getLogger(HealthController.class);

    private final JdbcTemplate jdbcTemplate;
    private final String version;

    public HealthController(JdbcTemplate jdbcTemplate) {
        this.jdbcTemplate = jdbcTemplate;
        String implVersion = HealthController.class.getPackage().getImplementationVersion();
        this.version = implVersion != null ? implVersion : "dev";
    }

    @GetMapping("/health")
    public Map<String, Object> health() {
        return Map.of(
                "status", "UP",
                "version", version,
                "db", checkDatabase(),
                "ts", Instant.now().toEpochMilli()
        );
    }

    private Map<String, Object> checkDatabase() {
        try {
            String pgVersion = jdbcTemplate.queryForObject("select version()", String.class);
            Integer migrations = jdbcTemplate.queryForObject(
                    "select count(*) from flyway_schema_history where success = true", Integer.class);
            return Map.of(
                    "status", "UP",
                    "version", pgVersion != null ? pgVersion.split(",")[0] : "unknown",
                    "migrations", migrations != null ? migrations : 0
            );
        } catch (RuntimeException e) {
            log.warn("Проверка БД не прошла: {}", e.getMessage());
            return Map.of("status", "DOWN", "error", e.getClass().getSimpleName());
        }
    }
}
