package kz.bardak.history.domain;

/** Состояние матча в базе (`04-db-schema.md`). */
public enum MatchRecordStatus {

    IN_PROGRESS,
    /** Игрок отвалился: таймер хода остановлен, ждём возвращения (§5.2). */
    PAUSED,
    FINISHED,
    /** Не вернулся за отведённое время — рейтинг не трогается. */
    ABORTED
}
