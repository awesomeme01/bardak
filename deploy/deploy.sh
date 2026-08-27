#!/usr/bin/env bash
#
# Выкатка на сервер: собрать образ, поднять связку, дождаться здоровья.
#
# ⭐ Версия — это git-хэш, а не «latest». Откатываться некуда, если все образы называются
# одинаково; здесь же rollback.sh просто берёт предыдущий тег.
#
# Запуск с самой машины:            deploy/deploy.sh
# Запуск удалённо (через ssh):      BARDAK_HOST=user@vps deploy/deploy.sh

set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(dirname "$here")"
cd "$root"

if [[ ! -f deploy/.env ]]; then
    echo "нет deploy/.env — скопируй deploy/.env.example и заполни" >&2
    exit 2
fi

version="${BARDAK_VERSION:-$(git rev-parse --short HEAD)}"
compose=(docker compose -f deploy/compose.prod.yaml --env-file deploy/.env)

if [[ -n "${BARDAK_HOST:-}" ]]; then
    # ⚠️ Собираем ЗДЕСЬ и отправляем образ: на VPS в 2 ГБ сборка фронта и Go — это отказ
    # по памяти в самый неподходящий момент.
    echo "▸ собираю образ bardak:$version"
    docker build --build-arg "VERSION=$version" -t "bardak:$version" -f back-go/Dockerfile .
    echo "▸ отправляю образ на $BARDAK_HOST"
    docker save "bardak:$version" | gzip | ssh "$BARDAK_HOST" 'gunzip | docker load'
    echo "▸ отправляю конфигурацию"
    ssh "$BARDAK_HOST" 'mkdir -p ~/bardak/deploy ~/bardak/back-go'
    scp deploy/compose.prod.yaml deploy/Caddyfile deploy/.env "$BARDAK_HOST:~/bardak/deploy/"
    echo "▸ поднимаю"
    ssh "$BARDAK_HOST" "cd ~/bardak && BARDAK_VERSION=$version docker compose -f deploy/compose.prod.yaml --env-file deploy/.env up -d --no-build"
    healthcheck_target="ssh $BARDAK_HOST"
else
    echo "▸ собираю и поднимаю локально, версия $version"
    BARDAK_VERSION="$version" "${compose[@]}" up -d --build
    healthcheck_target=""
fi

# ⚠️ Выкатка не считается удачной, пока сервер не ответил ЖИВЫМ. Иначе «задеплоил»
# означает всего лишь «контейнер запустился», а игроки уже видят белый экран.
echo "▸ жду здоровья"
for attempt in $(seq 1 30); do
    if [[ -n "$healthcheck_target" ]]; then
        body="$(ssh "$BARDAK_HOST" 'curl -fsS http://127.0.0.1:8088/api/health 2>/dev/null || true' || true)"
    else
        body="$(docker compose -f deploy/compose.prod.yaml --env-file deploy/.env exec -T app \
                wget -qO- http://127.0.0.1:8088/api/health 2>/dev/null || true)"
    fi
    if [[ "$body" == *'"status":"UP"'* ]]; then
        echo "✅ поднялось: $body"
        echo "$version" > deploy/.last-version
        exit 0
    fi
    sleep 2
done

echo "❌ сервер не ответил здоровым за минуту. Логи:" >&2
if [[ -n "$healthcheck_target" ]]; then
    ssh "$BARDAK_HOST" 'cd ~/bardak && docker compose -f deploy/compose.prod.yaml logs --tail=50 app' >&2
else
    "${compose[@]}" logs --tail=50 app >&2
fi
exit 1
