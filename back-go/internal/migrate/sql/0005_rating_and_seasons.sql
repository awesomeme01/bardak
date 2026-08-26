-- Рейтинг и сезоны. Схема — planning/04-db-schema.md, модель — planning/07-rating-system.md.

create table user_rating (
    user_id        uuid         primary key references users (id) on delete cascade,
    rating         numeric(8, 2) not null default 1000,
    -- deviation и volatility MVP не использует: считается простой Elo. Колонки заводятся
    -- сразу, чтобы переезд на Glicko-2 или OpenSkill не требовал миграции с потерей
    -- истории — пересчёт делается по rating_history в хронологическом порядке.
    deviation      numeric(8, 2) not null default 350,
    volatility     numeric(8, 5) not null default 0.06,
    matches_played integer       not null default 0,
    updated_at     timestamptz   not null default now()
);

comment on table user_rating is 'Текущий рейтинг. Считается по матчу, а не по раздаче';

create table rating_history (
    id              bigserial     primary key,
    user_id         uuid          not null references users (id) on delete cascade,
    match_id        uuid          not null references matches (id) on delete cascade,
    rating_before   numeric(8, 2) not null,
    rating_after    numeric(8, 2) not null,
    deviation_after numeric(8, 2) not null,
    place           smallint      not null,
    players_count   smallint      not null,
    created_at      timestamptz   not null default now(),
    unique (user_id, match_id)
);

create index idx_rating_history_user_time on rating_history (user_id, created_at desc);

comment on table rating_history is
    'Подробная история: по ней строится график и пересчитывается рейтинг, если формула изменится';

-- Сезоны закрываются вручную, а не по календарю (ADR-037): для узкого круга календарный
-- цикл — лишний источник дат, а закрывать сезон имеет смысл по числу сыгранных партий.
create table seasons (
    id         uuid        primary key,
    name       varchar(64) not null,
    started_at timestamptz not null default now(),
    closed_at  timestamptz,
    created_at timestamptz not null default now()
);

-- Открытый сезон ровно один: частичный уникальный индекс не даёт открыть второй.
create unique index idx_seasons_single_open on seasons (closed_at) where closed_at is null;

alter table rating_history
    add column season_id uuid references seasons (id);

comment on column rating_history.season_id is 'Сезон, в котором сыгран матч; null — вне сезонов';

insert into seasons (id, name)
values ('33333333-3333-3333-3333-333333333333', 'Первый сезон');
