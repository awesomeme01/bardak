package kz.bardak.lobby.domain;

/**
 * Состояние игрока за столом. {@code LEFT} — не удаление строки: место освобождается,
 * но факт участия остаётся, и по нему видно, кто уходил.
 */
public enum SeatState {

    JOINED,
    READY,
    LEFT
}
