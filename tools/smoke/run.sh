#!/usr/bin/env bash
#
# Дымовые проверки против ЖИВОГО сервера: составы 2–5 игроков и друзья.
#
# ⭐ Это не замена gradle check. Юнит-тесты проверяют движок в изоляции, интеграционные —
# слои по отдельности. Здесь проверяется всё вместе и по-настоящему: сокеты, протокол,
# база и правила в одном матче. Ровно так ловится «стол встал» — состояние, из которого
# ни один игрок не может сходить.
#
# ⚠️ Сервер должен быть уже поднят, а база — доступна. Скрипт заводит своих ботов и
# оставляет после себя их аккаунты и столы: это отладочная среда, а не прод.
#
#   tools/smoke/run.sh                # всё
#   tools/smoke/run.sh 3              # только состав на троих
#   BARDAK_URL=http://10.0.0.5:8088 tools/smoke/run.sh
set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
URL="${BARDAK_URL:-http://localhost:8088}"
STAMP="${STAMP:-$(date +%H%M%S)}"

if ! curl -sf -o /dev/null "$URL/api/health"; then
    echo "❌ Сервер не отвечает: $URL — подними его и повтори"
    exit 2
fi

# Node 20+: нужен встроенный WebSocket. На машине по умолчанию бывает старый.
if ! node -e 'process.exit(typeof WebSocket === "function" ? 0 : 1)' 2>/dev/null; then
    echo "❌ Нужен Node 20+ со встроенным WebSocket (сейчас $(node -v 2>/dev/null || echo 'нет node'))"
    exit 2
fi

failed=0

run() {
    echo
    echo "── $1 ──"
    shift
    if ! node "$@"; then
        failed=1
    fi
}

if [ $# -gt 0 ]; then
    run "состав $1" "$HERE/playmatch.mjs" "$1" "$STAMP"
else
    for players in 2 3 4 5; do
        run "состав $players" "$HERE/playmatch.mjs" "$players" "$STAMP"
    done
    run "друзья" "$HERE/friends.mjs" "$STAMP"
fi

echo
if [ "$failed" -eq 0 ]; then
    echo "✅ дымовые проверки прошли"
else
    echo "❌ есть провалившиеся проверки"
fi
exit "$failed"
