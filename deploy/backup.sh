#!/usr/bin/env bash
#
# Снять дамп базы и увезти его с сервера.
#
# ⭐ Формат -Fc (custom): он сжат и позволяет восстанавливать выборочно. Обычный SQL-текст
# на несколько сотен матчей раздувается и восстанавливается дольше.
#
# ⚠️ Бэкап, лежащий рядом с базой, спасает от «уронил таблицу» и не спасает от «потерял
# сервер». Поэтому копия уезжает в S3 — если он настроен; если нет, скрипт об этом ГОВОРИТ,
# а не делает вид, что всё хорошо.
#
# Крон на сервере:  15 4 * * *  /home/user/bardak/deploy/backup.sh >> /var/log/bardak-backup.log 2>&1

set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(dirname "$here")"
cd "$root"

# shellcheck disable=SC1091
[[ -f deploy/.env ]] && set -a && . deploy/.env && set +a

dir="${BARDAK_BACKUP_DIR:-$root/backups}"
mkdir -p "$dir"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
file="$dir/bardak-$stamp.dump"

echo "▸ снимаю дамп в $file"
docker compose -f deploy/compose.prod.yaml --env-file deploy/.env exec -T postgres \
    pg_dump -U bardak -d bardak -Fc > "$file"

# ⚠️ Пустой или обрезанный дамп — худший вид бэкапа: он есть, и он бесполезен.
size=$(wc -c < "$file")
if [[ "$size" -lt 4096 ]]; then
    echo "❌ дамп подозрительно мал ($size байт) — не считаю его бэкапом" >&2
    rm -f "$file"
    exit 1
fi
echo "  размер: $size байт"

if [[ -n "${BARDAK_BACKUP_BUCKET:-}" ]]; then
    echo "▸ увожу в $BARDAK_BACKUP_BUCKET"
    if command -v aws >/dev/null 2>&1; then
        aws s3 cp "$file" "$BARDAK_BACKUP_BUCKET/$(basename "$file")"
    elif command -v rclone >/dev/null 2>&1; then
        rclone copy "$file" "$BARDAK_BACKUP_BUCKET"
    else
        echo "❌ ни aws, ни rclone не установлены — дамп остался только на сервере" >&2
        exit 1
    fi
else
    echo "⚠️ BARDAK_BACKUP_BUCKET не задан: копия лежит ТОЛЬКО на этом сервере."
    echo "   Потеря сервера = потеря истории матчей и рейтинга."
fi

days="${BARDAK_BACKUP_RETENTION_DAYS:-14}"
echo "▸ чищу дампы старше $days дней"
find "$dir" -name 'bardak-*.dump' -type f -mtime "+$days" -print -delete

echo "✅ бэкап готов"
