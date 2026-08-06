# 04 — Схема базы данных

Черновик, готовый превратиться в Flyway-миграции. DDL ниже — набросок, а не финальный текст
миграции: типы и ограничения уточняются при реализации.

**Принципы:**

1. Flyway с первой миграции. Никакого `ddl-auto: update`.
2. `jsonb` — только для данных переменной формы, по которым мы **не ищем и не джойним**
   (payload события, конфиг правил стола). Всё, по чему фильтруем — обычные колонки.
3. `game_events` — append-only. Никаких `UPDATE`/`DELETE`.
4. Все временные метки — `timestamptz`.
5. Идентификаторы сущностей — `uuid`; порядковые вещи (лог событий) — `bigserial`.

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

## Рейтинг

```sql
create table user_rating (
    user_id       uuid primary key references users(id) on delete cascade,
    rating        numeric(8,2) not null default 1000,   -- текущее значение
    deviation     numeric(8,2) not null default 350,    -- неопределённость (RD / sigma)
    volatility    numeric(8,5) not null default 0.06,   -- для Glicko-2, пока не используется
    games_played  integer      not null default 0,
    updated_at    timestamptz  not null default now()
);

create table rating_history (
    id             bigserial primary key,
    user_id        uuid not null references users(id) on delete cascade,
    game_id        uuid not null references games(id) on delete cascade,
    rating_before  numeric(8,2) not null,
    rating_after   numeric(8,2) not null,
    deviation_after numeric(8,2) not null,
    place          smallint not null,
    players_count  smallint not null,
    created_at     timestamptz not null default now()
);
create index idx_rating_history_user_time on rating_history(user_id, created_at desc);
```

`deviation` и `volatility` заводятся **сразу**, хотя MVP использует простой Elo: переезд на
Glicko-2 / OpenSkill не должен требовать миграции с потерей данных. См. `07-rating-system.md`.

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
    card_code    varchar(16) not null,    -- 'S_A', 'H_10', 'JOKER_1', 'BACK'
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
    felt_color        varchar(16),         -- '#0b6623'
    default_back_code varchar(16),         -- какую рубашку использовать по умолчанию
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
    status         varchar(16)  not null,          -- WAITING | IN_GAME | CLOSED
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
    seat_no    smallint not null,          -- 0..4, определяет порядок хода
    state      varchar(16) not null,       -- JOINED | READY | LEFT
    joined_at  timestamptz not null default now(),
    primary key (table_id, user_id),
    unique (table_id, seat_no)
);
```

`version` на столе нужен для гонки «двое одновременно занимают последнее место».
`unique (table_id, seat_no)` закрывает ту же гонку на уровне БД — второй получит нарушение
ограничения и вежливый отказ.

`rules_config` в `jsonb` — переменной формы, зависит от того, какие правила включены:

```json
{
  "attackOrder": "BARDAK_STRICT_NEIGHBOURS",
  "transfersEnabled": true,
  "jokersEnabled": true,
  "naves": { "enabled": false },
  "turnTimeoutSeconds": 45,
  "maxCardsPerRound": 6
}
```

---

## Партии

```sql
create table games (
    id             uuid primary key,
    table_id       uuid not null references game_tables(id),
    status         varchar(16) not null,          -- IN_PROGRESS | FINISHED | ABORTED
    players_count  smallint    not null,
    rng_seed       bigint      not null,          -- детерминизм и реплей
    rules_snapshot jsonb       not null,          -- копия rules_config на момент старта
    trump_suit     varchar(2),                    -- 'S'|'H'|'D'|'C'
    started_at     timestamptz not null default now(),
    finished_at    timestamptz,
    loser_user_id  uuid references users(id)      -- «дурак»
);
create index idx_games_table_time on games(table_id, started_at desc);

create table game_players (
    game_id        uuid not null references games(id) on delete cascade,
    user_id        uuid not null references users(id),
    seat_no        smallint not null,
    place          smallint,                      -- 1 = вышел первым; null пока идёт
    rating_before  numeric(8,2),
    rating_after   numeric(8,2),
    rating_delta   numeric(8,2),
    primary key (game_id, user_id)
);
create index idx_game_players_user on game_players(user_id);
```

`rules_snapshot` — обязателен. Правила стола могут поменяться позже; партия должна остаться
интерпретируемой ровно по тем правилам, по которым игралась. Без этого реплей старых партий
сломается при первом же изменении правил.

`rng_seed` не отдаётся клиенту, пока партия не завершена.

---

## Лог событий и снапшоты ⭐

Основа истории, реплея и восстановления стола после рестарта.

```sql
create table game_events (
    id          bigserial primary key,
    game_id     uuid        not null references games(id) on delete cascade,
    seq         integer     not null,        -- порядковый номер внутри партии, с 1
    type        varchar(32) not null,        -- CARD_PLAYED, TRICK_RESOLVED, ...
    actor_seat  smallint,                    -- null для системных событий
    payload     jsonb       not null,
    created_at  timestamptz not null default now(),
    unique (game_id, seq)
);
create index idx_game_events_game on game_events(game_id, seq);

create table game_snapshots (
    game_id    uuid    not null references games(id) on delete cascade,
    seq        integer not null,       -- состояние ПОСЛЕ события с этим seq
    state      jsonb   not null,       -- полное внутреннее состояние партии
    created_at timestamptz not null default now(),
    primary key (game_id, seq)
);
```

Правила работы с логом:

- Только `INSERT`. Событие записано — оно неизменяемо.
- `unique (game_id, seq)` гарантирует отсутствие дыр и дублей при гонках.
- Запись события происходит **до** рассылки клиентам (см. `02-architecture.md`).
- `payload` содержит **полную** информацию, включая скрытую (какая именно карта у кого) —
  это внутренний лог. Наружу он никогда не отдаётся сырым, только через проекцию.
- Снапшот пишется раз в N событий (например, каждые 50) — чтобы восстановление стола и
  переподключение не требовали проигрывания всей партии.

Партиционирование `game_events` по времени — когда таблица вырастет (🟢, не на MVP).

---

## Порядок миграций (M1–M6)

| Версия | Что |
|---|---|
| `V1__users_and_auth.sql` | `users`, `refresh_tokens` |
| `V2__card_sets_and_themes.sql` | `card_sets`, `card_assets`, `table_themes` + дефолтный набор |
| `V3__tables.sql` | `game_tables`, `table_players` |
| `V4__games_and_events.sql` | `games`, `game_players`, `game_events`, `game_snapshots` |
| `V5__rating.sql` | `user_rating`, `rating_history` |

---

## Открытые решения

- Хранить ли текущее состояние активного стола в БД, или только лог + память.
  Сейчас: **только лог + память**, состояние восстанавливается. Пересмотреть, если
  восстановление окажется медленным.
- Мягкое удаление пользователей (`status = BLOCKED` против физического `DELETE`) — сейчас
  мягкое, потому что на пользователя ссылается история партий.
