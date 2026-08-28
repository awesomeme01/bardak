#!/bin/sh
# Убрать за ботами: дымы, differential и нагрузочные прогоны плодят учётки, столы
# и матчи в общей базе. Живых игроков скрипт не касается — снос строго по маскам
# логинов, которыми боты и создаются (см. tools/smoke/*.mjs, tests/contract/*.mjs).
#
# Запуск: tools/smoke/cleanup.sh
set -e

docker exec -i bardak-postgres psql -U bardak -d bardak -v ON_ERROR_STOP=1 <<'SQL'
begin;

-- Маски: smokeN_ (playmatch), loadN_ (loadtest), frNNN (friends), md_ (matchdiff),
-- sd_/sm_ (socketdiff), ct- (compare), rt_/svgtest_ (разовые проверки).
create temp table doomed as
  select id from users
  where username ~ '^(smoke[2-5]_|load[2-5]_|fr[0-9]|md_|sd_|sm_|rt_|svgtest_|ct-)';

create temp table doomed_tables as
  select id from game_tables where host_user_id in (select id from doomed);

-- Порядок продиктован внешними ключами без каскада: сначала матчи ботских столов
-- (раздачи, журнал, снимки, история — каскадом), потом сами столы, потом люди.
delete from matches where table_id in (select id from doomed_tables);
delete from matches where loser_user_id in (select id from doomed);
delete from table_players where user_id in (select id from doomed);
delete from match_players where user_id in (select id from doomed);
delete from friendships where low_user_id in (select id from doomed)
   or high_user_id in (select id from doomed) or requested_by in (select id from doomed);
delete from game_tables where id in (select id from doomed_tables);
delete from users where id in (select id from doomed);

select count(*) as "снесено ботов" from doomed;
commit;
SQL

docker exec bardak-postgres psql -U bardak -d bardak -c \
  "select (select count(*) from users) as \"осталось игроков\",
          (select count(*) from game_tables) as \"столов\",
          (select count(*) from matches) as \"матчей\";"
