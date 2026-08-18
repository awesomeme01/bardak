package kz.bardak.common.web;

import jakarta.servlet.http.HttpServletRequest;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.UUID;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.MethodArgumentNotValidException;
import org.springframework.web.bind.annotation.ExceptionHandler;
import org.springframework.web.bind.annotation.RestControllerAdvice;
import org.springframework.web.servlet.resource.NoResourceFoundException;

/**
 * Превращает исключения в единый формат ответа.
 *
 * <p>⚠️ Наружу уходит только код и заранее подготовленное сообщение. Текст исключения
 * в тело ответа не попадает: в нём легко оказаться деталям вроде имени таблицы.
 */
@RestControllerAdvice
public class ApiExceptionHandler {

    private static final Logger log = LoggerFactory.getLogger(ApiExceptionHandler.class);

    @ExceptionHandler(ApiException.class)
    public ResponseEntity<ApiError> handleApi(final ApiException exception) {
        final String traceId = newTraceId();
        log.info("Запрос отклонён [{}] {}: {}", traceId, exception.code(), exception.getMessage());
        return ResponseEntity.status(exception.status())
                .body(new ApiError(exception.code(), exception.getMessage(), traceId, exception.details()));
    }

    @ExceptionHandler(MethodArgumentNotValidException.class)
    public ResponseEntity<ApiError> handleValidation(final MethodArgumentNotValidException exception) {
        final Map<String, Object> fields = new LinkedHashMap<>();
        exception.getBindingResult().getFieldErrors()
                .forEach(error -> fields.put(error.getField(), error.getDefaultMessage()));
        final String traceId = newTraceId();
        log.info("Невалидный запрос [{}]: {}", traceId, fields);
        return ResponseEntity.badRequest()
                .body(new ApiError("VALIDATION_FAILED", "Проверьте заполнение полей", traceId, fields));
    }

    /**
     * Неизвестный путь.
     *
     * <p>⚠️ Без этого обработчика любой опечатанный или снесённый адрес под {@code /api}
     * отвечал <b>500</b> и писал в лог полный стек как «необработанная ошибка»: Spring
     * бросает {@link NoResourceFoundException}, и она проваливалась в общий обработчик.
     * Клиент не мог отличить «такого адреса нет» от «сервер сломался», а лог наполнялся
     * стеками там, где ничего не ломалось.
     */
    @ExceptionHandler(NoResourceFoundException.class)
    public ResponseEntity<ApiError> handleNotFound(final HttpServletRequest request) {
        final String traceId = newTraceId();
        log.info("Неизвестный путь [{}]: {}", traceId, request.getRequestURI());
        return ResponseEntity.status(HttpStatus.NOT_FOUND)
                .body(new ApiError("NOT_FOUND", "Такого адреса нет", traceId, Map.of()));
    }

    @ExceptionHandler(Exception.class)
    public ResponseEntity<ApiError> handleUnexpected(final Exception exception, final HttpServletRequest request) {
        final String traceId = newTraceId();
        log.error("Необработанная ошибка [{}] на {}", traceId, request.getRequestURI(), exception);
        return ResponseEntity.status(HttpStatus.INTERNAL_SERVER_ERROR)
                .body(new ApiError("INTERNAL_ERROR", "Что-то пошло не так", traceId, Map.of()));
    }

    private String newTraceId() {
        return UUID.randomUUID().toString().substring(0, 8);
    }
}
