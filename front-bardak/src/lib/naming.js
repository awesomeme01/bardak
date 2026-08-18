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

/**
 * Мордочка игрока — постоянная, выведенная из его идентификатора.
 *
 * Не случайная и не хранимая: случайная менялась бы при каждом входе, а хранить картинку
 * ради узнавания за столом на пятерых — заводить целую подсистему там, где хватает
 * остатка от деления.
 */
export const FACES = ['🦊', '🥔', '🗿', '😼', '🐻', '🦉', '🐙', '🦅', '🐺', '🦁', '🐸', '🦄'];

export function avatarOf(userId, chosen = null) {
    if (chosen) {
        return chosen;
    }
    if (!userId) {
        return '🃏';
    }
    let sum = 0;
    for (let i = 0; i < userId.length; i++) {
        sum = (sum * 31 + userId.charCodeAt(i)) % 100000;
    }
    return FACES[sum % FACES.length];
}

/** Масть одним значком: в тесном месте название не помещается. */
const SUIT_GLYPHS = {DIAMONDS: '♦', HEARTS: '♥', SPADES: '♠', CLUBS: '♣'};

export function suitGlyph(code) {
    return SUIT_GLYPHS[code] ?? '—';
}

/** Красные масти красим — иначе на сукне ♦ и ♥ неотличимы от ♠ и ♣. */
export function isRedSuit(code) {
    return code === 'DIAMONDS' || code === 'HEARTS';
}

/** Короткая запись карты: «6♦» вместо «6-diamonds», «🃏» вместо «Joker-2». */
export function shortCard(code) {
    if (!code) {
        return '';
    }
    if (code.startsWith('Joker')) {
        return '🃏';
    }
    const [rank, suit] = code.split('-');
    const glyph = {diamonds: '♦', hearts: '♥', spades: '♠', clubs: '♣'}[suit] ?? '';
    return rank + glyph;
}

/**
 * Событие матча человеческой фразой.
 *
 * <p>⭐ Реплей рассказывает партию, а не показывает лог. `CARD_ATTACKED` с кодом карты —
 * это протокол; человек хочет прочитать «кладёт 7♦». Поэтому здесь перевод, а не вывод
 * типа события на экран.
 *
 * @param nameOf кто сидит на месте — имя игрока по номеру места
 */
export function replayLine(event, nameOf) {
    const p = event.payload ?? {};
    const card = shortCard(p.cardCode);
    switch (event.type) {
        case 'CARD_ATTACKED': return `кладёт ${card}`;
        case 'CARD_DEFENDED': return `бьёт ${shortCard(p.targetCardCode)} картой ${card}`;
        case 'ATTACK_TRANSFERRED': return `переводит ${card} на ${nameOf(p.toSeatNo)}`;
        case 'PASSED': return 'пасует';
        case 'ATTACK_RIGHT_MOVED': return 'право подкидывать уходит дальше';
        case 'TAKE_ANNOUNCED': return 'объявляет «беру»';
        case 'CARDS_TAKEN': return `забирает стол — ${p.count} ${cards(p.count)}`;
        case 'ROUND_BEATEN': return `бито — ${p.count} ${cards(p.count)} в отбой`;
        case 'CARDS_DRAWN': return `добирает ${p.count} ${cards(p.count)}`;
        case 'CARD_HUNG': return `навешивает ${card} игроку ${nameOf(p.victimSeat)}`;
        case 'NAVES_LEVEL_CHANGED': return 'поднимается на ступень по шкале';
        case 'HANGING_WINDOW_OPENED': return 'открылось окно навеса';
        case 'HANGING_WINDOW_CLOSED': return 'окно навеса закрылось';
        case 'HIDDEN_TRUMP_REVEALED': return `вскрывает потайной козырь ${card}`;
        case 'TRUMP_CHANGED': return `козырь меняется на ${suitName(p.suit)}`;
        case 'TRUMP_CHOSEN': return `называет козырь ${suitName(p.suit)}`;
        case 'FACE_DOWN_REVEALED': return `вскрывает скрытую карту ${card}`;
        case 'DICE_ROLLED': return 'бросок кости за право хода';
        case 'PLAYER_LEFT_DEAL': return 'выходит из раздачи — карт не осталось';
        case 'DEAL_FINISHED': return 'раздача окончена';
        case 'MOVE_REJECTED': return 'попытка хода отклонена правилами';
        default: return event.type.toLowerCase().replaceAll('_', ' ');
    }
}

function cards(count) {
    const tail = count % 10;
    const hundred = count % 100;
    if (tail === 1 && hundred !== 11) {
        return 'карту';
    }
    if (tail >= 2 && tail <= 4 && (hundred < 12 || hundred > 14)) {
        return 'карты';
    }
    return 'карт';
}
