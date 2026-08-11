package kz.bardak.lobby.domain;

/** Жизненный цикл стола (`04-db-schema.md`). */
public enum TableStatus {

    /** Ждёт игроков, за него можно сесть. */
    WAITING,

    /** Идёт матч: новые игроки не садятся. */
    IN_MATCH,

    /** Закрыт хостом или доигран. */
    CLOSED
}
