#!/usr/bin/env bash
#
# Откат на предыдущий образ.
#
# ⚠️ Откатывается ТОЛЬКО приложение. База не трогается: миграции идут вперёд, и старый
# бинарник на новой схеме работает, а «откат схемы» превращает аварию в потерю данных.
# Если сломала именно миграция — это разбор руками, а не одна кнопка.

set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(dirname "$here")"
cd "$root"

target="${1:-}"
if [[ -z "$target" ]]; then
    echo "Доступные образы:"
    docker images bardak --format '  {{.Tag}}\t{{.CreatedSince}}'
    echo
    echo "Запуск: deploy/rollback.sh <тег>"
    exit 2
fi

if ! docker image inspect "bardak:$target" >/dev/null 2>&1; then
    echo "образа bardak:$target нет на этой машине" >&2
    exit 1
fi

echo "▸ откатываюсь на $target"
BARDAK_VERSION="$target" docker compose -f deploy/compose.prod.yaml --env-file deploy/.env up -d --no-build app

for attempt in $(seq 1 30); do
    body="$(docker compose -f deploy/compose.prod.yaml --env-file deploy/.env exec -T app \
            wget -qO- http://127.0.0.1:8088/api/health 2>/dev/null || true)"
    if [[ "$body" == *'"status":"UP"'* ]]; then
        echo "✅ откат состоялся: $body"
        exit 0
    fi
    sleep 2
done

echo "❌ и откат не ответил здоровым — смотри логи" >&2
exit 1
