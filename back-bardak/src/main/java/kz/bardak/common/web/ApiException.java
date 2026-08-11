package kz.bardak.common.web;

import java.util.Map;
import java.util.Objects;
import org.springframework.http.HttpStatus;

/**
 * Ошибка, которую можно показать клиенту. Несёт машиночитаемый код — по нему фронт решает,
 * что делать, а текст может меняться (`05-api-contracts.md`).
 */
public class ApiException extends RuntimeException {

    private final String code;
    private final HttpStatus status;
    private final transient Map<String, Object> details;

    public ApiException(final HttpStatus status, final String code, final String message) {
        this(status, code, message, Map.of());
    }

    public ApiException(final HttpStatus status, final String code, final String message,
                        final Map<String, Object> details) {
        super(Objects.requireNonNull(message, "message"));
        this.status = Objects.requireNonNull(status, "status");
        this.code = Objects.requireNonNull(code, "code");
        this.details = Map.copyOf(Objects.requireNonNull(details, "details"));
    }

    public String code() {
        return code;
    }

    public HttpStatus status() {
        return status;
    }

    public Map<String, Object> details() {
        return details;
    }
}
