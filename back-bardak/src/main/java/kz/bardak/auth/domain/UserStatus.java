package kz.bardak.auth.domain;

/**
 * Состояние учётной записи. Удаления пользователей нет: на них ссылается история матчей
 * (`04-db-schema.md`), поэтому «удалённый» — это {@link #BLOCKED}.
 */
public enum UserStatus {

    ACTIVE,
    BLOCKED
}
