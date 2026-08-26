-- Подписки на push-уведомления «твой ход».
--
-- ⭐ Подписка принадлежит устройству, а не пользователю: телефон и ноутбук — две разные
-- строки, и уведомление уходит на оба. Ключ — endpoint, его выдаёт push-сервис браузера.
create table push_subscriptions (
    id         uuid         primary key,
    user_id    uuid         not null references users (id) on delete cascade,
    endpoint   varchar(512) not null unique,
    p256dh     varchar(128) not null,
    auth       varchar(64)  not null,
    user_agent varchar(256),
    created_at timestamptz  not null default now(),
    last_sent_at timestamptz
);

create index idx_push_subscriptions_user on push_subscriptions (user_id);

comment on table push_subscriptions is
    'Подписки на уведомления; endpoint выдаёт push-сервис браузера и он же уникален';
comment on column push_subscriptions.p256dh is
    'Открытый ключ подписки: полезная нагрузка шифруется им (RFC 8291), сервер её содержимого не хранит';
