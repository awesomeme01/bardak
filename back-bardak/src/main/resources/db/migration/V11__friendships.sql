-- Друзья.
--
-- ⭐ Дружба хранится ОДНОЙ строкой на пару, а не двумя зеркальными. Две строки надо держать
-- согласованными при каждом изменении, и рассинхрон здесь означает, что человек у тебя в
-- друзьях, а ты у него нет. Поэтому пара нормализуется: в low_user_id всегда меньший из
-- двух идентификаторов, и уникальность пары обеспечивает индекс, а не дисциплина кода.
--
-- ⭐ Заявка и дружба — одна и та же строка в разных состояниях. Отдельная таблица заявок
-- потребовала бы переноса строки при принятии, то есть удаления и вставки там, где хватает
-- смены статуса.

create table friendships (
    low_user_id    uuid        not null references users (id),
    high_user_id   uuid        not null references users (id),
    -- Кто позвал: по нему видно, чья заявка висит и кому её принимать.
    requested_by   uuid        not null references users (id),
    status         varchar(16) not null check (status in ('PENDING', 'ACCEPTED')),
    created_at     timestamptz not null default now(),
    decided_at     timestamptz,
    primary key (low_user_id, high_user_id),
    -- Дружить с самим собой нельзя: это не запрет, а бессмыслица.
    constraint friendships_not_self check (low_user_id <> high_user_id),
    -- Нормализация пары держится проверкой, а не соглашением в коде.
    constraint friendships_ordered check (low_user_id < high_user_id)
);

create index idx_friendships_high on friendships (high_user_id);

comment on table friendships is 'Дружба и заявки: одна строка на пару, порядок идентификаторов нормализован';
comment on column friendships.requested_by is 'Кто отправил заявку — принимать её должен другой';
