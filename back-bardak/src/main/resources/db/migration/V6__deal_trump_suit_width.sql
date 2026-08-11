-- Козырь раздачи пишется именем масти движка (DIAMONDS, HEARTS, SPADES, CLUBS), а не
-- значком: значок красив в psql, но разбирается по таблице соответствий, которой у истории
-- нет. Двух символов под имя не хватает.
alter table deals
    alter column trump_suit type varchar(16);

comment on column deals.trump_suit is
    'Имя масти движка; null — раздача кончилась до того, как козырь назвали';
