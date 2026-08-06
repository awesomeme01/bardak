-- Пользователи и авторизация.
-- Схема описана в planning/04-db-schema.md.

create table users (
    id            uuid         primary key,
    username      varchar(32)  not null unique,
    display_name  varchar(64)  not null,
    email         varchar(255) unique,
    password_hash varchar(255) not null,
    avatar_url    varchar(512),
    status        varchar(16)  not null default 'ACTIVE',
    created_at    timestamptz  not null default now(),
    updated_at    timestamptz  not null default now(),
    constraint users_status_check check (status in ('ACTIVE', 'BLOCKED'))
);

comment on table users is 'Игроки';
comment on column users.display_name is 'Имя, видимое за столом';
comment on column users.password_hash is 'BCrypt';

-- Храним хеш refresh-токена, а не сам токен: утечка дампа БД не должна давать
-- возможность войти за пользователя. То же соображение, что и с паролями.
create table refresh_tokens (
    id         uuid         primary key,
    user_id    uuid         not null references users (id) on delete cascade,
    token_hash varchar(255) not null unique,
    expires_at timestamptz  not null,
    revoked_at timestamptz,
    user_agent varchar(255),
    created_at timestamptz  not null default now()
);

create index idx_refresh_tokens_user
    on refresh_tokens (user_id)
    where revoked_at is null;
