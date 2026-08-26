-- Столы, наборы карт и темы. Схема описана в planning/04-db-schema.md.

create table card_sets (
    id            uuid         primary key,
    code          varchar(64)  not null unique,
    name          varchar(128) not null,
    description   text,
    version       varchar(16)  not null,
    preview_url   varchar(512),
    is_default    boolean      not null default false,
    is_public     boolean      not null default true,
    owner_user_id uuid references users (id),
    created_at    timestamptz  not null default now()
);

-- Набор по умолчанию ровно один: частичный уникальный индекс не даёт завести второй.
create unique index idx_card_sets_single_default on card_sets (is_default) where is_default;

comment on table card_sets is 'Наборы дизайна карт; движок о них не знает (ADR-009)';
comment on column card_sets.owner_user_id is 'null — системный набор';

create table card_assets (
    id          uuid         primary key,
    card_set_id uuid         not null references card_sets (id) on delete cascade,
    card_code   varchar(24)  not null,
    asset_url   varchar(512) not null,
    mime        varchar(64)  not null,
    ordinal     smallint     not null default 0,
    unique (card_set_id, card_code)
);

comment on column card_assets.card_code is 'Код карты движка: 6-diamonds, 10-hearts, Joker, back';

create table table_themes (
    id                uuid         primary key,
    code              varchar(64)  not null unique,
    name              varchar(128) not null,
    background_url    varchar(512),
    felt_color        varchar(16),
    default_back_code varchar(16),
    preview_url       varchar(512),
    is_default        boolean      not null default false,
    created_at        timestamptz  not null default now()
);

create unique index idx_table_themes_single_default on table_themes (is_default) where is_default;

create table game_tables (
    id           uuid        primary key,
    code         varchar(8)  not null unique,
    name         varchar(64) not null,
    host_user_id uuid        not null references users (id),
    max_players  smallint    not null,
    status       varchar(16) not null,
    card_set_id  uuid        not null references card_sets (id),
    theme_id     uuid        not null references table_themes (id),
    rules_config jsonb       not null default '{}'::jsonb,
    is_private   boolean     not null default false,
    version      integer     not null default 0,
    created_at   timestamptz not null default now(),
    closed_at    timestamptz,
    constraint game_tables_status_check check (status in ('WAITING', 'IN_MATCH', 'CLOSED')),
    constraint game_tables_max_players_check check (max_players between 2 and 5)
);

create index idx_game_tables_open on game_tables (status) where status = 'WAITING';

comment on column game_tables.code is 'Короткий код приглашения';
comment on column game_tables.version is 'Оптимистичная блокировка: двое занимают последнее место';

create table table_players (
    table_id  uuid        not null references game_tables (id) on delete cascade,
    user_id   uuid        not null references users (id),
    seat_no   smallint    not null,
    state     varchar(16) not null,
    joined_at timestamptz not null default now(),
    primary key (table_id, user_id),
    -- ⭐ Та же гонка за последнее место, закрытая на уровне БД: второй получит нарушение
    -- ограничения и вежливый отказ, даже если проверка в коде отработала одновременно.
    unique (table_id, seat_no),
    constraint table_players_state_check check (state in ('JOINED', 'READY', 'LEFT')),
    constraint table_players_seat_check check (seat_no between 0 and 4)
);

comment on column table_players.seat_no is 'Порядок хода по часовой; фиксирован на весь матч';

-- Набор по умолчанию. Картинки лежат в back-bardak/assets и отдаются как /assets/**,
-- поэтому в манифесте только пути, а не сами файлы.
insert into card_sets (id, code, name, description, version, preview_url, is_default)
values ('11111111-1111-1111-1111-111111111111', 'classic', 'Классический',
        'Обычная колода, PNG 500×726', '1.0.0',
        '/assets/card-sets/classic/A-spades.png', true);

-- Карты генерируются перебором, а не 54 строками руками: список рангов и мастей и так
-- зафиксирован кодами движка, а ручной список — это 54 возможности опечататься.
insert into card_assets (id, card_set_id, card_code, asset_url, mime, ordinal)
select gen_random_uuid(),
       '11111111-1111-1111-1111-111111111111',
       rank.code || '-' || suit.code,
       '/assets/card-sets/classic/' || rank.code || '-' || suit.code || '.png',
       'image/png',
       rank.ordinal * 10 + suit.ordinal
from (values ('2', 1), ('3', 2), ('4', 3), ('5', 4), ('6', 5), ('7', 6), ('8', 7),
             ('9', 8), ('10', 9), ('J', 10), ('Q', 11), ('K', 12), ('A', 13)) as rank(code, ordinal)
cross join (values ('diamonds', 1), ('hearts', 2), ('spades', 3), ('clubs', 4)) as suit(code, ordinal);

insert into card_assets (id, card_set_id, card_code, asset_url, mime, ordinal)
values (gen_random_uuid(), '11111111-1111-1111-1111-111111111111', 'Joker',
        '/assets/card-sets/classic/Joker.png', 'image/png', 200),
       (gen_random_uuid(), '11111111-1111-1111-1111-111111111111', 'back',
        '/assets/card-sets/classic/back.png', 'image/png', 201);

insert into table_themes (id, code, name, felt_color, default_back_code, is_default)
values ('22222222-2222-2222-2222-222222222222', 'green-felt', 'Зелёное сукно',
        '#1f6f43', 'back', true);
