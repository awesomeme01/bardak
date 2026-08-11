-- Матчи, раздачи и лог событий. Схема описана в planning/04-db-schema.md.
-- Лог — основа истории, реплея и восстановления стола после рестарта (ADR-004).

create table matches (
    id             uuid        primary key,
    table_id       uuid        not null references game_tables (id),
    status         varchar(16) not null,
    players_count  smallint    not null,
    deals_played   smallint    not null default 0,
    rng_seed       bigint      not null,
    rules_snapshot jsonb       not null,
    started_at     timestamptz not null default now(),
    finished_at    timestamptz,
    loser_user_id  uuid references users (id),
    abort_reason   varchar(255),
    constraint matches_status_check check (status in ('IN_PROGRESS', 'PAUSED', 'FINISHED', 'ABORTED'))
);

create index idx_matches_table_time on matches (table_id, started_at desc);
create index idx_matches_status on matches (status) where status in ('IN_PROGRESS', 'PAUSED');

comment on column matches.rules_snapshot is
    'Копия rules_config на момент старта: правила стола могут поменяться, а матч обязан остаться интерпретируемым';
comment on column matches.rng_seed is 'Не отдаётся клиенту, пока матч не завершён: по нему вычисляется вся колода';
comment on column matches.loser_user_id is 'Главный проигравший; остальные — в match_players.loss_type';

create table match_players (
    match_id      uuid     not null references matches (id) on delete cascade,
    user_id       uuid     not null references users (id),
    seat_no       smallint not null,
    naves_level   varchar(2),
    loss_type     varchar(24),
    place         smallint,
    rating_before numeric(8, 2),
    rating_after  numeric(8, 2),
    rating_delta  numeric(8, 2),
    primary key (match_id, user_id),
    constraint match_players_loss_type_check check (loss_type is null or loss_type in (
        'ROYAL', 'SUPER_MEGA_SUCK', 'SUPER_MEGA_FAIL', 'SUPER_FAIL', 'FAIL'
    ))
);

create index idx_match_players_user on match_players (user_id);

comment on column match_players.naves_level is
    'Счёта в очках нет: роль счёта играет уровень навеса. null — «летит 6»';

create table deals (
    id                uuid        primary key,
    match_id          uuid        not null references matches (id) on delete cascade,
    deal_no           smallint    not null,
    trump_suit        varchar(2),
    started_at        timestamptz not null default now(),
    finished_at       timestamptz,
    loser_seat        smallint,
    last_attack_cards jsonb       not null default '[]'::jsonb,
    unique (match_id, deal_no)
);

comment on column deals.last_attack_cards is
    'Что было выложено в последней атаке, а не что попало в руку: от этого зависят степени ROYAL и SUPER_MEGA_SUCK';

create table deal_results (
    deal_id            uuid     not null references deals (id) on delete cascade,
    seat_no            smallint not null,
    place              smallint,
    hung_cards         jsonb    not null default '[]'::jsonb,
    naves_level_before varchar(2),
    naves_level_after  varchar(2),
    level_changes      jsonb    not null default '[]'::jsonb,
    primary key (deal_id, seat_no)
);

comment on column deal_results.level_changes is
    'Почему уровень поменялся: сдвигов четыре вида и они сочетаются в одной раздаче';

create table match_events (
    id         bigserial   primary key,
    match_id   uuid        not null references matches (id) on delete cascade,
    seq        integer     not null,
    deal_no    smallint,
    type       varchar(32) not null,
    actor_seat smallint,
    payload    jsonb       not null,
    created_at timestamptz not null default now(),
    -- Гарантия отсутствия дыр и дублей при гонках: только INSERT, seq сквозной по матчу.
    unique (match_id, seq)
);

create index idx_match_events_match on match_events (match_id, seq);

comment on table match_events is 'Append-only лог. Событие записано — оно неизменяемо (ADR-004)';
comment on column match_events.payload is
    'Полная информация, включая скрытую: это внутренний лог. Наружу отдаётся только через проекцию';

create table match_snapshots (
    match_id   uuid        not null references matches (id) on delete cascade,
    seq        integer     not null,
    state      jsonb       not null,
    created_at timestamptz not null default now(),
    primary key (match_id, seq)
);

comment on table match_snapshots is
    'Состояние ПОСЛЕ события с этим seq. Пишется на границе раздач — там оно компактно';
