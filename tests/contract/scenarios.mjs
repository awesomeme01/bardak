/**
 * Сценарии контрактной проверки: одни и те же запросы к двум бэкендам.
 *
 * ⭐ Сценарий описывает НАМЕРЕНИЕ, а не готовый запрос: идентификаторы рождаются по ходу
 * (зарегистрировались — получили токен — создали стол), и захардкодить их нельзя.
 * Поэтому шаг — это функция от накопленного состояния.
 *
 * ⚠️ Сравнивается ответ ПОСЛЕ нормализации: идентификаторы, время и колода у двух
 * бэкендов заведомо разные, и сверять их бессмысленно. Всё остальное обязано совпасть
 * побайтно — коды, имена полей, их наличие и отсутствие.
 */

const PASSWORD = 'very-secret-password';
const INVITE = 'bardak-2026';

/** Уникальный логин на прогон: базы у бэкендов общие, и повтор упёрся бы в занятый логин. */
const uniq = (prefix, run) => `${prefix}${run}`;

export const scenarios = [
    {
        name: 'health',
        steps: [
            {what: 'GET /api/health', request: () => ({method: 'GET', path: '/api/health'})},
        ],
    },
    {
        name: 'регистрация и профиль',
        steps: [
            {
                what: 'POST /api/auth/register',
                request: ({run}) => ({
                    method: 'POST', path: '/api/auth/register',
                    body: {username: uniq('ct-reg-', run), displayName: 'Контракт',
                           password: PASSWORD, inviteCode: INVITE},
                }),
                keep: (state, body) => ({...state, token: body.accessToken}),
            },
            {
                what: 'GET /api/profile',
                request: ({token}) => ({method: 'GET', path: '/api/profile', token}),
            },
            {
                what: 'GET /api/profile без токена',
                request: () => ({method: 'GET', path: '/api/profile'}),
            },
        ],
    },
    {
        name: 'ошибки регистрации',
        steps: [
            {
                what: 'неверный код приглашения',
                request: ({run}) => ({
                    method: 'POST', path: '/api/auth/register',
                    body: {username: uniq('ct-bad-', run), displayName: 'Х',
                           password: PASSWORD, inviteCode: 'нет-такого'},
                }),
            },
            {
                what: 'короткий пароль — ошибка валидации',
                request: ({run}) => ({
                    method: 'POST', path: '/api/auth/register',
                    body: {username: uniq('ct-short-', run), displayName: 'Х',
                           password: '123', inviteCode: INVITE},
                }),
            },
            {
                what: 'битое тело',
                request: () => ({method: 'POST', path: '/api/auth/login', raw: '{это не json'}),
            },
            {
                what: 'неизвестный путь',
                request: () => ({method: 'GET', path: '/api/такого-нет'}),
            },
        ],
    },
    {
        name: 'столы',
        steps: [
            {
                what: 'регистрация хозяина',
                request: ({run}) => ({
                    method: 'POST', path: '/api/auth/register',
                    body: {username: uniq('ct-host-', run), displayName: 'Хозяин',
                           password: PASSWORD, inviteCode: INVITE},
                }),
                keep: (state, body) => ({...state, token: body.accessToken}),
            },
            {
                what: 'POST /api/tables',
                request: ({token}) => ({
                    method: 'POST', path: '/api/tables', token,
                    body: {name: 'Контрактный', maxPlayers: 3, rulesConfig: {}, isPrivate: false},
                }),
                keep: (state, body) => ({...state, tableId: body.id, code: body.code}),
            },
            {
                what: 'GET /api/tables/current',
                request: ({token}) => ({method: 'GET', path: '/api/tables/current', token}),
            },
            {
                what: 'GET /api/tables/invite/{code} без токена',
                request: ({code}) => ({method: 'GET', path: `/api/tables/invite/${code}`}),
            },
            {
                what: 'GET /api/tables/invite/ZZZZZZ — нет такого',
                request: () => ({method: 'GET', path: '/api/tables/invite/ZZZZZZ'}),
            },
            {
                what: 'GET /api/tables/{кривой-uuid}',
                request: ({token}) => ({method: 'GET', path: '/api/tables/не-uuid', token}),
            },
            {
                what: 'второй стол тем же игроком',
                request: ({token}) => ({
                    method: 'POST', path: '/api/tables', token,
                    body: {name: 'Второй', maxPlayers: 2, rulesConfig: {}, isPrivate: false},
                }),
            },
        ],
    },
    {
        name: 'рейтинг и статистика новичка',
        steps: [
            {
                what: 'регистрация',
                request: ({run}) => ({
                    method: 'POST', path: '/api/auth/register',
                    body: {username: uniq('ct-rook-', run), displayName: 'Новичок',
                           password: PASSWORD, inviteCode: INVITE},
                }),
                keep: (state, body) => ({...state, token: body.accessToken}),
            },
            {
                what: 'GET /api/rating/me — рейтинга ещё нет',
                request: ({token}) => ({method: 'GET', path: '/api/rating/me', token}),
            },
            {
                what: 'GET /api/stats/me — пустая статистика',
                request: ({token}) => ({method: 'GET', path: '/api/stats/me', token}),
            },
            {
                what: 'GET /api/rating/seasons',
                request: ({token}) => ({method: 'GET', path: '/api/rating/seasons', token}),
            },
            {
                what: 'GET /api/rating/top',
                request: ({token}) => ({method: 'GET', path: '/api/rating/top', token}),
            },
            {
                what: 'GET /api/matches — своя история',
                request: ({token}) => ({method: 'GET', path: '/api/matches', token}),
            },
        ],
    },
    {
        name: 'друзья',
        steps: [
            {
                what: 'регистрация',
                request: ({run}) => ({
                    method: 'POST', path: '/api/auth/register',
                    body: {username: uniq('ct-fr-', run), displayName: 'Друг',
                           password: PASSWORD, inviteCode: INVITE},
                }),
                keep: (state, body) => ({...state, token: body.accessToken}),
            },
            {
                what: 'GET /api/friends — пусто',
                request: ({token}) => ({method: 'GET', path: '/api/friends', token}),
            },
            {
                what: 'заявка несуществующему',
                request: ({token}) => ({
                    method: 'POST', path: '/api/friends/requests', token,
                    body: {username: 'нет-такого-логина'},
                }),
            },
        ],
    },
    {
        name: 'каталоги',
        steps: [
            {what: 'GET /api/card-sets', request: () => ({method: 'GET', path: '/api/card-sets'})},
            {what: 'GET /api/table-themes', request: () => ({method: 'GET', path: '/api/table-themes'})},
        ],
    },
];
