# 04 — Схема базы данных

Черновик, готовый превратиться в Flyway-миграции. DDL ниже — набросок, а не финальный текст
миграции: типы и ограничения уточняются при реализации.

**Принципы:**

1. Flyway с первой миграции. Никакого `ddl-auto: update`.
2. `jsonb` — только для данных переменной формы, по которым мы **не ищем и не джойним**
   (payload события, конфиг правил). Всё, по чему фильтруем — обычные колонки.
3. `match_events` — append-only. Никаких `UPDATE`/`DELETE`.
4. Все временные метки — `timestamptz`.
5. Идентификаторы сущностей — `uuid`; порядковые вещи (лог событий) — `bigserial`.

**Терминология** (см. `03-domain-rules.md` §0):
**матч** = игра в бардак до чьего-то джокера, **раздача** = одна партия дурака внутри матча.
Рейтинг и история строятся вокруг **матча**.

---

## Пользователи и авторизация

```sql
create table users (
    id             uuid primary key,
    username       varchar(32)  not null unique,      -- логин, латиница, нижний регистр
    display_name   varchar(64)  not null,             -- имя за столом
    email          varchar(255) unique,               -- nullable: на MVP можно без почты
    password_hash  varchar(255) not null,             -- BCrypt
    avatar_url     varchar(512),
    status         varchar(16)  not null,             -- ACTIVE | BLOCKED
    created_at     timestamptz  not null default now(),
    updated_at     timestamptz  not null default now()
);

create table refresh_tokens (
    id          uuid primary key,
    user_id     uuid not null references users(id) on delete cascade,
    token_hash  varchar(255) not null unique,   -- храним хеш, не сам токен
    expires_at  timestamptz  not null,
    revoked_at  timestamptz,
    user_agent  varchar(255),
    created_at  timestamptz not null default now()
);
create index idx_refresh_tokens_user on refresh_tokens(user_id) where revoked_at is null;
```

Почему хеш токена, а не токен: утечка дампа БД не должна давать возможность войти за
пользователя. То же соображение, что и с паролями.

---

## Наборы карт и темы стола

```sql
create table card_sets (
    id            uuid primary key,
    code          varchar(64) not null unique,   -- 'classic', 'neon', ...
    name          varchar(128) not null,
    description   text,
    version       varchar(16)  not null,         -- semver манифеста
    preview_url   varchar(512),
    is_default    boolean not null default false,
    is_public     boolean not null default true,
    owner_user_id uuid references users(id),     -- null = системный набор
    created_at    timestamptz not null default now()
);
create unique index idx_card_sets_single_default on card_sets(is_default) where is_default;

create table card_assets (
    id           uuid primary key,
    card_set_id  uuid not null references card_sets(id) on delete cascade,
    card_code    varchar(24) not null,    -- '6-diamonds', '10-hearts', 'Joker', 'back'
    asset_url    varchar(512) not null,
    mime         varchar(64)  not null,   -- image/svg+xml | image/png
    ordinal      smallint     not null default 0,
    unique (card_set_id, card_code)
);

create table table_themes (
    id                uuid primary key,
    code              varchar(64) not null unique,
    name              varchar(128) not null,
    background_url    varchar(512),
    felt_color        varchar(16),
    default_back_code varchar(16),
    preview_url       varchar(512),
    is_default        boolean not null default false,
    created_at        timestamptz not null default now()
);
```

Подробности модели — `06-card-design-system.md`.

---

## Столы

```sql
create table game_tables (
    id             uuid primary key,
    code           varchar(8)   not null unique,   -- короткий код для приглашения
    name           varchar(64)  not null,
    host_user_id   uuid not null references users(id),
    max_players    smallint     not null,          -- 2..5
    status         varchar(16)  not null,          -- WAITING | IN_MATCH | CLOSED
    card_set_id    uuid not null references card_sets(id),
    theme_id       uuid not null references table_themes(id),
    rules_config   jsonb        not null default '{}'::jsonb,
    is_private     boolean      not null default false,
    version        integer      not null default 0,   -- оптимистичная блокировка
    created_at     timestamptz  not null default now(),
    closed_at      timestamptz
);
create index idx_game_tables_open on game_tables(status) where status = 'WAITING';

create table table_players (
    table_id   uuid not null references game_tables(id) on delete cascade,
    user_id    uuid not null references users(id),
    seat_no    smallint not null,          -- 0..4, определяет порядок хода по часовой
    state      varchar(16) not null,       -- JOINED | READY | LEFT
    joined_at  timestamptz not null default now(),
    primary key (table_id, user_id),
    unique (table_id, seat_no)
);
```

`version` на столе нужен для гонки «двое одновременно занимают последнее место».
`unique (table_id, seat_no)` закрывает ту же гонку на уровне БД — второй получит нарушение
ограничения и вежливый отказ.

### `rules_config` ⭐

Все игровые числа живут здесь, а не в коде (`03-domain-rules.md` §1.6):

```json
{
  "dealSize": 6,
  "maxAttackFirstRound": 5,
  "maxAttackPerRound": 6,
  "turnTimeoutSeconds": 30,
  "disconnectGraceSeconds": 60,
  "attackOrder": "BARDAK_STRICT_NEIGHBOURS",
  "transfersEnabled": true,
  "jokersEnabled": true,
  "showRejectedAttempts": false,
  "naves": {
    "enabled": false,
    "scale": ["6", "7", "8", "9", "10", "J", "Q", "K", "A"],
    "finalCard": "JOKER"
  }
}
```

`naves.scale` — та самая шкала из `03-domain-rules.md` §0.1, вынесенная в конфиг по ADR-016.
Её длина определяет, сколько навесов игрок выдерживает до проигрыша; сокращённая шкала даёт
более быстрый матч.

Форма переменная и зависит от включённых правил — поэтому `jsonb`, а не колонки.

---

## Матчи ⭐

```sql
create table matches (
    id             uuid primary key,
    table_id       uuid not null references game_tables(id),
    status         varchar(16) not null,     -- IN_PROGRESS | PAUSED | FINISHED | ABORTED
    players_count  smallint    not null,
    deals_played   smallint    not null default 0,
    rng_seed       bigint      not null,     -- под-seed раздачи выводится из него
    rules_snapshot jsonb       not null,     -- копия rules_config на момент старта
    started_at     timestamptz not null default now(),
    finished_at    timestamptz,
    -- ⭐ главный проигравший: тот, у кого степень старше по шкале.
    -- Проигравших может быть несколько — все они в match_players.loss_type.
    loser_user_id  uuid references users(id),
    abort_reason   varchar(255)                 -- 'Игрок 3 покинул игру'
);
create index idx_matches_table_time on matches(table_id, started_at desc);
create index idx_matches_status on matches(status) where status in ('IN_PROGRESS','PAUSED');

create table match_players (
    match_id       uuid not null references matches(id) on delete cascade,
    user_id        uuid not null references users(id),
    seat_no        smallint not null,
    naves_level    varchar(2),     -- ⭐ '6'..'A', 'JK'; null = «летит 6», навесов не было
    loss_type      varchar(24),    -- ⭐ степень проигрыша; null = не проиграл
    place          smallint,                      -- итоговое место, null пока идёт
    rating_before  numeric(8,2),
    rating_after   numeric(8,2),
    rating_delta   numeric(8,2),
    primary key (match_id, user_id),
    constraint match_players_loss_type_check check (loss_type is null or loss_type in (
        'ROYAL', 'SUPER_MEGA_SUCK', 'SUPER_MEGA_FAIL', 'SUPER_FAIL', 'FAIL'
    ))
);
create index idx_match_players_user on match_players(user_id);
```

Ключевые моменты:

- ⭐ **`naves_level` вместо `score`.** В бардаке нет очков: роль счёта играет уровень навеса —
  личная шкала `6 → 7 → … → A → Joker` (`03-domain-rules.md` §0.1). Он живёт на уровне матча,
  переносится между раздачами и умеет **уменьшаться**: выход первым даёт `−1`.
- Нет колонки `score_limit`: предел один и он не настраивается — это конец шкалы, джокер.
- ⭐ **`loss_type` у игрока, а не только у матча.** Проигравших может быть несколько, у
  каждого своя степень (`03-domain-rules.md` §0.3). В `matches.loser_user_id` — только
  главный, тот, у кого степень старше.

Порядок степеней от тяжёлой к лёгкой: `ROYAL` → `SUPER_MEGA_SUCK` → `SUPER_MEGA_FAIL` →
`SUPER_FAIL` → `FAIL`. В БД это `varchar` с `check`, а не `enum`: правила ещё уточняются,
а менять `check` дешевле, чем тип. Порядок задаётся кодом — сортировать степени средствами
SQL нам не нужно.
- `rules_snapshot` **обязателен**. Правила стола могут поменяться позже; матч должен остаться
  интерпретируемым ровно по тем правилам, по которым игрался. Без этого реплей старых матчей
  сломается при первом же изменении правил.
- `ABORTED` матчи **не влияют на рейтинг**: `rating_*` остаются `null`, строк в
  `rating_history` не появляется. Но матч сохраняется целиком и доступен для просмотра.
- `rng_seed` не отдаётся клиенту, пока матч не завершён.

Почему `naves_level` — `varchar`, а не число: ранги это домен игры, и `'J'`, `'Q'`, `'K'`
читаются в дампе и в логе без расшифровки. Порядок задаётся кодом, а не сортировкой БД —
сортировать шкалу навесов средствами SQL нам нигде не нужно.

---

## Раздачи

```sql
create table deals (
    id           uuid primary key,
    match_id     uuid not null references matches(id) on delete cascade,
    deal_no      smallint not null,          -- 1, 2, 3, ... внутри матча
    trump_suit   varchar(2),                 -- 'S'|'H'|'D'|'C'
    started_at   timestamptz not null default now(),
    finished_at  timestamptz,
    loser_seat   smallint,                   -- «дурак» этой раздачи
    -- ⭐ состав последней атаки: от него зависят степени ROYAL и SUPER_MEGA_SUCK
    last_attack_cards jsonb not null default '[]'::jsonb,
    unique (match_id, deal_no)
);

create table deal_results (
    deal_id            uuid not null references deals(id) on delete cascade,
    seat_no            smallint not null,
    place              smallint,       -- порядок выхода; null = проиграл раздачу
    hung_cards         jsonb not null default '[]'::jsonb,  -- что навесили картами
    naves_level_before varchar(2),
    naves_level_after  varchar(2),
    -- ⭐ откуда взялся сдвиг: разбор по причинам, а не только итог
    level_changes      jsonb not null default '[]'::jsonb,
    primary key (deal_id, seat_no)
);
```

`deal_results` отвечает на вопрос «как игрок дошёл до своего уровня»: по нему строится
разбор матча по раздачам.

`hung_cards` — карты, навешенные игроку **в этой раздаче** (`["6-diamonds", "7-clubs"]`).
После раздачи они возвращаются в колоду, и кроме лога их взять негде.

⭐ `last_attack_cards` в `deals` — карты, выложенные в атаке последнего раунда
(`["8-clubs","8-hearts","8-spades","8-diamonds"]`). Считается именно **выложенное**, а не
то, что попало проигравшему в руку: он мог всё отбить и всё равно получить `ROYAL`
(`03-domain-rules.md` §0.3).

⭐ `level_changes` — почему уровень поменялся. Сдвигов четыре вида и они могут сочетаться
в одной раздаче (`03-domain-rules.md` §0.1), поэтому одного «было → стало» мало:

```json
[
  {"reason": "HUNG",           "by": 2, "card": "7-clubs", "to": "7"},
  {"reason": "LOST_DEAL",      "delta": 1,  "to": "8"},
  {"reason": "FINISHED_FIRST", "delta": -1, "to": "7"},
  {"reason": "KILLED_LOSER",   "delta": -1, "victim": 3, "to": "6"}
]
```

Без этого разбор спорной ситуации превращается в гадание — особенно вокруг
`KILLED_LOSER`, где `−1` прилетает за чужой проигрыш.

---

## Лог событий и снапшоты ⭐

Основа истории, реплея и восстановления стола после рестарта.

```sql
create table match_events (
    id          bigserial   primary key,
    match_id    uuid        not null references matches(id) on delete cascade,
    seq         integer     not null,        -- сквозной номер по МАТЧУ, с 1
    deal_no     smallint,                    -- null для событий уровня матча
    type        varchar(32) not null,        -- CARD_PLAYED, TRICK_RESOLVED, CARD_HUNG...
    actor_seat  smallint,                    -- null для системных событий
    payload     jsonb       not null,
    created_at  timestamptz not null default now(),
    unique (match_id, seq)
);
create index idx_match_events_match on match_events(match_id, seq);

create table match_snapshots (
    match_id   uuid    not null references matches(id) on delete cascade,
    seq        integer not null,       -- состояние ПОСЛЕ события с этим seq
    state      jsonb   not null,       -- полное внутреннее состояние: матч + текущая раздача
    created_at timestamptz not null default now(),
    primary key (match_id, seq)
);
```

Почему `seq` сквозной по матчу, а не по раздаче: клиент отслеживает **один** счётчик за всё
время за столом, и `RESYNC(lastSeq)` остаётся простым — не нужно синхронизировать пару
`(dealNo, seq)` и обрабатывать переход между раздачами как особый случай. `deal_no` в строке
события есть, поэтому разбить лог по раздачам всегда можно запросом.

Правила работы с логом:

- Только `INSERT`. Событие записано — оно неизменяемо.
- `unique (match_id, seq)` гарантирует отсутствие дыр и дублей при гонках.
- Запись события происходит **до** рассылки клиентам (см. `02-architecture.md`).
- `payload` содержит **полную** информацию, включая скрытую (какая карта у кого) — это
  внутренний лог. Наружу он никогда не отдаётся сырым, только через проекцию.
- Отклонённые попытки хода (§2.1 правил) в лог **пишутся** — они часть истории стола,
  хотя состояние не меняют.
- Снапшот пишется раз в N событий (например, каждые 50) и **обязательно на границе раздач** —
  граница это естественная точка, где состояние компактно.

Партиционирование `match_events` по времени — когда таблица вырастет (🟢, не на MVP).

---

## Рейтинг

```sql
create table user_rating (
    user_id       uuid primary key references users(id) on delete cascade,
    rating        numeric(8,2) not null default 1000,
    deviation     numeric(8,2) not null default 350,    -- Glicko-2 RD / OpenSkill σ, про запас
    volatility    numeric(8,5) not null default 0.06,   -- про запас
    matches_played integer     not null default 0,
    updated_at    timestamptz  not null default now()
);

create table rating_history (
    id              bigserial primary key,
    user_id         uuid not null references users(id) on delete cascade,
    match_id        uuid not null references matches(id) on delete cascade,
    rating_before   numeric(8,2) not null,
    rating_after    numeric(8,2) not null,
    deviation_after numeric(8,2) not null,
    place           smallint not null,
    players_count   smallint not null,
    created_at      timestamptz not null default now()
);
create index idx_rating_history_user_time on rating_history(user_id, created_at desc);
```

Считается **по матчу**, не по раздаче: рейтинговое событие — получение кем-то джокера и
счёта. Отменённые матчи сюда не попадают вообще.

`deviation` и `volatility` заводятся сразу, хотя MVP использует простой Elo — переезд на
Glicko-2 / OpenSkill не должен требовать миграции с потерей данных. См. `07-rating-system.md`.

---

## Порядок миграций (M1–M6)

| Версия | Что |
|---|---|
| `V1__users_and_auth.sql` | `users`, `refresh_tokens` |
| `V2__card_sets_and_themes.sql` | `card_sets`, `card_assets`, `table_themes` + дефолтный набор |
| `V3__tables.sql` | `game_tables`, `table_players` |
| `V4__matches_and_deals.sql` | `matches`, `match_players`, `deals`, `deal_results` |
| `V5__match_events.sql` | `match_events`, `match_snapshots` |
| `V6__rating.sql` | `user_rating`, `rating_history` |

---

## Открытые решения

- Хранить ли текущее состояние активного матча в БД, или только лог + память.
  Сейчас: **только лог + снапшоты + память**. Пересмотреть, если восстановление окажется
  медленным.
- Мягкое удаление пользователей (`status = BLOCKED` вместо `DELETE`) — на пользователя
  ссылается история матчей.
- ✅ `loss_type` становится перечислением: `ROYAL`, `SUPER_MEGA_SUCK`, `SUPER_MEGA_FAIL`,
  `SUPER_FAIL`, `FAIL` (§0.3). Ничьей нет — в каждой раздаче ровно один проигравший
  (ADR-033).
- ⬜ Таблица `seasons` (id, начало, конец, статус) и ссылка на сезон в рейтинге — сезоны
  закрываются вручную (ADR-037).
