/**
 * Названия для человека.
 *
 * Названия степеней в интерфейсе авторские, а в коде и в базе — нейтральные константы
 * (§0.3). Перевод живёт в одном месте: «SUPER_MEGA_FAIL», просочившийся на экран, читается
 * как ошибка, а не как результат партии.
 */

const DEGREES = {
    ROYAL: 'Королевский',
    SUPER_MEGA_SUCK: 'Супер-мега-сак',
    SUPER_MEGA_FAIL: 'Супер-мега-фейл',
    SUPER_FAIL: 'Супер-фейл',
    FAIL: 'Фейл',
};

const REASONS = {
    LOST_DEAL: 'проиграл раздачу',
    FIRST_OUT: 'вышел первым',
    FINISHED_OPPONENT: 'добил соперника',
    SCALE_LIMIT: 'край шкалы',
};

const SUITS = {
    DIAMONDS: '♦ бубны',
    HEARTS: '♥ черви',
    SPADES: '♠ пики',
    CLUBS: '♣ трефы',
};

export function degreeName(code) {
    return DEGREES[code] ?? code;
}

export function reasonName(code) {
    return REASONS[code] ?? code;
}

export function suitName(code) {
    return SUITS[code] ?? '—';
}

/** Уровень навесов. Пусто — навесов не было вовсе: «летит 6», а не ступень шкалы. */
export function levelName(code) {
    if (code === null || code === undefined) {
        return 'летит 6';
    }
    return code === 'Jk' ? 'джокер' : code;
}

/** Дельта рейтинга со знаком: «+20.0» читается лучше, чем «20». */
export function deltaText(value) {
    if (value === null || value === undefined) {
        return '—';
    }
    const number = Number(value);
    return number > 0 ? `+${number.toFixed(1)}` : number.toFixed(1);
}
