-- Игрок сидит за одним столом за раз.
--
-- ⚠️ Раньше это правило существовало только в намерении: уникальный индекс стоял на
-- (table_id, seat_no), то есть защищал место от двух игроков, но не игрока от двух мест.
-- Пять одновременных «Создать стол» заводили пять столов и сажали за них одного человека —
-- каждый запрос успевал проверить «сижу ли я где-то» раньше, чем сосед вставлял строку.
--
-- Проверкой в коде такую гонку не закрыть: между чтением и вставкой всегда есть щель.
-- Закрывает её только база.

delete from table_players p
 where exists (select 1
                 from table_players other
                where other.user_id = p.user_id
                  and (other.joined_at, other.table_id) < (p.joined_at, p.table_id));

create unique index ux_table_players_user on table_players (user_id);

comment on index ux_table_players_user is
    'Игрок сидит за одним столом за раз; гонку «создать стол дважды» ловит именно этот индекс';
