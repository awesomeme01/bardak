#!/usr/bin/env bash
#
# Выкатка на сервер: залить исходники, собрать образ ТАМ, перезапустить приложение.
#
# ⭐ Собираем на сервере, а не у себя: на рабочей машине корпоративный TLS-перехват
# ломает установку пакетов внутри контейнера (`apk` не доверяет подменённому
# сертификату). На VPS перехвата нет — там сборка проходит как задумано.
#
# ⚠️⚠️ ГЛАВНОЕ ПРАВИЛО ЭТОГО ФАЙЛА: `deploy/.env` живёт ТОЛЬКО на сервере и в rsync
# не участвует. В репозитории его нет (там секреты), поэтому `--delete` считает его
# лишним и удаляет — вместе с паролем базы, ключом JWT и парой VAPID. Один раз это
# уже случилось: спасло лишь то, что контейнеры были живы и переменные удалось
# вычитать из них через `docker inspect`. Второй раз может не спасти.
#
# Запуск: tools/ship.sh [user@host]

set -euo pipefail

host="${1:-${BARDAK_HOST:-root@93.171.232.185}}"
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(dirname "$here")"
cd "$root"

version="$(git rev-parse --short HEAD)"
dirty="$(git status --porcelain | head -1)"
if [[ -n "$dirty" ]]; then
    echo "⚠️ в рабочем дереве есть незакоммиченные правки — версия $version соврёт" >&2
fi

echo "▸ заливаю исходники на $host (версия $version)"
rsync -az --delete \
    --exclude 'deploy/.env' \
    --exclude node_modules --exclude .git \
    --exclude 'front-bardak/dist' \
    --exclude back-bardak --exclude planning --exclude docs \
    -e "ssh -o BatchMode=yes" \
    back-go front-bardak assets deploy tools tests README.md \
    "$host:~/bardak/"

# ⚠️ Проверка после заливки, а не вместо неё: если .env всё-таки исчез, лучше узнать
# об этом здесь, чем при перезапуске уже остановленного приложения.
if ! ssh -o BatchMode=yes "$host" 'test -s ~/bardak/deploy/.env'; then
    echo "❌ на сервере нет deploy/.env — выкатку не продолжаю" >&2
    echo "   восстанови его из работающих контейнеров: docker inspect bardak-app-1" >&2
    exit 1
fi

echo "▸ собираю образ на сервере"
ssh -o BatchMode=yes "$host" "cd ~/bardak && docker build --build-arg VERSION=$version \
    -t bardak:$version -t bardak:latest -f back-go/Dockerfile . > /root/build.log 2>&1" || {
    echo "❌ сборка упала, хвост лога:" >&2
    ssh -o BatchMode=yes "$host" 'tail -25 /root/build.log' >&2
    exit 1
}

echo "▸ перезапускаю приложение"
ssh -o BatchMode=yes "$host" "cd ~/bardak
    sed -i 's/^BARDAK_VERSION=.*/BARDAK_VERSION=$version/' deploy/.env
    docker compose -f deploy/compose.prod.yaml --env-file deploy/.env up -d --no-build app"

# ⭐ «Задеплоил» означает «отвечает», а не «контейнер запустился».
echo -n "▸ жду здоровья: "
for _ in $(seq 1 30); do
    live="$(ssh -o BatchMode=yes "$host" \
        "docker exec bardak-app-1 wget -qO- http://127.0.0.1:8088/api/health 2>/dev/null" || true)"
    if [[ "$live" == *'"status":"UP"'* ]]; then
        echo "UP, версия $(echo "$live" | sed -n 's/.*"version":"\([^"]*\)".*/\1/p')"
        exit 0
    fi
    sleep 2
done

echo "❌ приложение не ответило UP" >&2
ssh -o BatchMode=yes "$host" 'cd ~/bardak && docker compose -f deploy/compose.prod.yaml --env-file deploy/.env logs app --tail 30' >&2
exit 1
