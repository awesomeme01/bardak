package kz.bardak.common.web;

import com.fasterxml.jackson.annotation.JsonInclude;
import java.util.Map;

/**
 * Единый формат ошибки для всех эндпоинтов (`05-api-contracts.md`).
 *
 * @param code    машиночитаемый и стабильный
 * @param message для человека, может меняться
 * @param traceId по нему ошибка ищется в логах
 * @param details подробности, если они безопасны для показа
 */
@JsonInclude(JsonInclude.Include.NON_EMPTY)
public record ApiError(String code, String message, String traceId, Map<String, Object> details) {
}
