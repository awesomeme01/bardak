# Дымовые проверки

Боты играют настоящие матчи против живого сервера — по сокетам, через протокол, с базой.

```sh
docker compose up -d
cd back-bardak && JAVA_HOME=$(/usr/libexec/java_home -v 21) ./gradlew bootRun   # в отдельном окне

tools/smoke/run.sh          # составы 2–5 и друзья
tools/smoke/run.sh 5        # только пятеро
```

Нужен **Node 20+**: используется встроенный `WebSocket`. На машине по умолчанию активен
Node 10 — `export PATH="$HOME/.nvm/versions/node/v22.23.2/bin:$PATH"`.

## Зачем они, если есть `gradle check`

`check` проверяет движок в изоляции и слои по отдельности. Здесь всё работает вместе и
по-настоящему: сокет, протокол, база, правила, таймеры. Так ловятся поломки, которых
не видно ни в одном юнит-тесте:

- **стол встал** — состояние, из которого ни один игрок не может сходить, а матч не
  закончен. Именно так зависала раздача, когда спасовавший сохранял право хода;
- **`availableActions` разошлись с движком** — бот ходит только тем, что сервер сам объявил
  разрешённым, поэтому расхождение сразу видно по всплеску отказов;
- **приглашение не доходит** — присутствие и доставка проверяются на живых сокетах.

## Что печатают

```
✅ 4 игрока(ов): матч завершён, ходов 923
   события: CARD_ATTACKED:1932 PASSED:1296 CARD_DEFENDED:1116 …
   отказы: NOT_YOUR_TURN:28 CARD_NOT_IN_HAND:2
```

**Отказы — норма.** Бот считает ход по снимку, который успевает устареть, пока летит:
чем больше игроков, тем чаще. Смотреть надо на коды, а не на число.

| Код | Что значит |
|---|---|
| `NOT_YOUR_TURN`, `CARD_NOT_IN_HAND` | гонка бота, ожидаемо |
| что угодно другое | разбираться: сервер предложил ход, который сам же не принял |

Строка `события` — беглая проверка, что состав действительно прошёл через механики, а не
отыграл три хода: у трёх и более игроков должны появиться `ATTACK_RIGHT_MOVED` и
`HANGING_WINDOW_OPENED`, иначе подкидывание и навесы остались непроверенными.

## Нагрузочная проверка (M9)

Отвечает на вопрос «сколько столов держит один узел» — тот самый, ради которого мы
отказались от Redis и второго инстанса (ADR-061).

```sh
tools/smoke/loadtest.mjs ramp        # лестница 2 → 5 → 10 → 20 → 40 столов
tools/smoke/loadtest.mjs 10 4        # 10 столов по 4 игрока
THINK_MS=0 tools/smoke/loadtest.mjs 5   # без пауз: потолок железа, НЕ ёмкость
```

⭐ **Меряется задержка хода, а не пропускная способность.** Сколько тысяч ходов в секунду
выжмет машина — игроку не видно; видно, сколько он ждёт после нажатия. Предел узла — это
момент, когда ожидание перестаёт читаться как мгновенное (бюджет p95 — 400 мс), а вовсе
не момент, когда сервер падает.

⚠️ **Боты думают перед ходом** (`THINK_MS`, по умолчанию ~1.2 с с разбросом). Без паузы
стол генерирует ходы в сотни раз чаще живого, и «сорок столов» такого прогона не имеют
ничего общего с сорока живыми столами. Пауза — условие того, чтобы число столов
что-то значило, а не вежливость к серверу.

Конца матча прогон не ждёт: партия с человеческой паузой идёт много минут и сотни ходов,
а нужен установившийся режим. Замеряется окно (`WINDOW_MS`, минута).

Задержка считается как «команда → следующий снимок у этого же бота». Точной привязки
ответа к команде в протоколе нет — `STATE_SYNC` уходит всем сразу, — но именно эта
величина и есть ожидание игрока.

## После себя

Скрипты оставляют аккаунты ботов (`smoke*`, `fr*`) и их столы. Это отладочная среда, и
подчищать их автоматически нельзя: по ним же и разбираются упавшие прогоны. Убрать вручную:

Нагрузочный прогон заводит своих — с префиксом `load`, и их заметно больше: лестница до
160 столов оставляет под тысячу аккаунтов и столько же столов.

⚠️ Удалить одних пользователей не выйдет — на них висит история матчей (`match_players`,
`rating_history`), и `delete from users` упрётся во внешний ключ. Сносить надо от истории
к аккаунту, одной транзакцией:

```sql
begin;
create temp table bots as select id from users where username like 'load%';
create temp table botmatches as
  select m.id from matches m
   where m.table_id in (select id from game_tables where name like 'нагрузка%')
      or m.id in (select match_id from match_players where user_id in (select id from bots));

delete from deal_results where deal_id in (select id from deals where match_id in (select id from botmatches));
delete from deals           where match_id in (select id from botmatches);
delete from match_events    where match_id in (select id from botmatches);
delete from match_snapshots where match_id in (select id from botmatches);
delete from rating_history  where match_id in (select id from botmatches) or user_id in (select id from bots);
delete from match_players   where match_id in (select id from botmatches) or user_id in (select id from bots);
delete from matches         where id in (select id from botmatches);
delete from table_players   where user_id in (select id from bots);
delete from game_tables     where name like 'нагрузка%' or host_user_id in (select id from bots);
delete from push_subscriptions where user_id in (select id from bots);
delete from refresh_tokens  where user_id in (select id from bots);
delete from user_rating     where user_id in (select id from bots);
delete from users           where id in (select id from bots);
commit;
```

Для `smoke%` и `fr%` — то же самое с другим префиксом; у них объём небольшой.
