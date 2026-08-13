package kz.bardak.auth.api;

import jakarta.validation.constraints.Email;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.Size;

/**
 * Формы запросов и ответов авторизации (`05-api-contracts.md`).
 *
 * <p>Собраны в одном месте намеренно: это плоские DTO границы, и раскладывать их
 * по отдельным файлам — только мешать чтению контракта.
 */
public final class AuthDtos {

    private AuthDtos() {
    }

    public record RegisterRequest(
            @NotBlank @Size(min = 3, max = 32) String username,
            @NotBlank @Size(min = 2, max = 64) String displayName,
            @NotBlank @Size(min = 8, max = 128) String password,
            @Email @Size(max = 255) String email,
            @NotBlank String inviteCode) {
    }

    public record LoginRequest(@NotBlank String username, @NotBlank String password) {
    }

    public record RefreshRequest(@NotBlank String refreshToken) {
    }

    /**
     * @param accessToken  живёт минуты, хранится на клиенте только в памяти
     * @param refreshToken живёт долго; при каждом обмене выдаётся новый
     * @param expiresIn    сколько секунд осталось access-токену
     */
    public record TokenPair(String accessToken, String refreshToken, long expiresIn, UserView user) {
    }

    /** Публичное представление игрока: ни хеша пароля, ни почты. */
    public record UserView(String id, String username, String displayName, String avatarUrl,
                           String avatar) {
    }

    /** Одноразовый тикет для WS-рукопожатия (ADR-005). */
    /** Что можно про себя поменять. Логин не меняется: по нему входят. */
    public record UpdateProfileRequest(@NotBlank @Size(min = 2, max = 64) String displayName,
                                       @Size(max = 8) String avatar) {
    }

    /**
     * Смена пароля.
     *
     * <p>⚠️ Старый пароль обязателен даже при живом токене: украденная вкладка не должна
     * давать возможность запереть владельца снаружи.
     */
    public record ChangePasswordRequest(@NotBlank String currentPassword,
                                        @NotBlank @Size(min = 8, max = 128) String newPassword) {
    }

    public record WsTicket(String ticket, long expiresIn) {
    }
}
