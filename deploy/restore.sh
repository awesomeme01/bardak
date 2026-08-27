#!/usr/bin/env bash
#
# Восстановление из дампа — и учебная тревога (drill).
#
# ⭐ Умеет две вещи, и вторая важнее первой:
#   restore.sh drill <дамп>   — поднять дамп в ОТДЕЛЬНОЙ базе и проверить, что он живой;
#   restore.sh apply <дамп>   — восстановить БОЕВУЮ базу (со всеми последствиями).
#
# ⚠️ Бэкап, который ни разу не восстанавливали, — это не бэкап, а надежда. Drill гоняется
# на копии и ничего не ломает, поэтому гонять его можно и нужно регулярно.

set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(dirname "$here")"
cd "$root"

mode="${1:-}"
dump="${2:-}"
compose=(docker compose -f deploy/compose.prod.yaml --env-file deploy/.env)

if [[ -z "$mode" || -z "$dump" || ! -f "$dump" ]]; then
    echo "Запуск: deploy/restore.sh drill|apply <файл дампа>" >&2
    exit 2
fi

case "$mode" in
drill)
    probe="bardak_drill_$(date -u +%s)"
    echo "▸ учебная тревога: восстанавливаю в базу $probe"
    "${compose[@]}" exec -T postgres psql -U bardak -d postgres -c "create database $probe" >/dev/null
    # ⚠️ --no-owner: дамп снят под тем же пользователем, но в drill-базе прав может не хватить,
    # и падение здесь означало бы «бэкап плохой», хотя плох был бы только владелец объектов.
    "${compose[@]}" exec -T postgres pg_restore -U bardak -d "$probe" --no-owner < "$dump" >/dev/null

    echo "▸ проверяю, что восстановилось не пусто"
    read -r users matches deals <<<"$("${compose[@]}" exec -T postgres psql -U bardak -d "$probe" -t -A -F' ' \
        -c "select (select count(*) from users), (select count(*) from matches), (select count(*) from deals)")"
    echo "  пользователей: $users, матчей: $matches, раздач: $deals"

    "${compose[@]}" exec -T postgres psql -U bardak -d postgres -c "drop database $probe" >/dev/null
    if [[ "$users" -eq 0 ]]; then
        echo "❌ в дампе нет ни одного пользователя — это не рабочая копия" >&2
        exit 1
    fi
    echo "✅ дамп восстанавливается и содержит данные"
    ;;
apply)
    echo "⚠️ БОЕВОЕ восстановление: текущая база будет заменена содержимым дампа."
    read -r -p "Введи ДА для продолжения: " confirm
    [[ "$confirm" == "ДА" ]] || { echo "отменено"; exit 1; }

    echo "▸ останавливаю приложение, чтобы оно не писало в базу во время восстановления"
    "${compose[@]}" stop app
    "${compose[@]}" exec -T postgres psql -U bardak -d postgres -c \
        "drop database if exists bardak with (force); create database bardak" >/dev/null
    "${compose[@]}" exec -T postgres pg_restore -U bardak -d bardak --no-owner < "$dump"
    "${compose[@]}" start app
    echo "✅ восстановлено; проверь /api/health и историю матчей"
    ;;
*)
    echo "неизвестный режим: $mode" >&2
    exit 2
    ;;
esac
