#!/usr/bin/env bash
#
# Прокси для Telegram рядом с игрой: поднять/обновить и напечатать ссылку.
#
# ⭐ Секрет живёт в deploy/.env на сервере, как и остальные. Нет его — выпускается
# на MTG_DOMAIN (или BARDAK_DOMAIN) и дописывается в .env (перед этим .env копируется в .env.bak).
# ⚠️ Файл .env не перезаписывается целиком — только дописывается: см. README про аварию.
#
# Запуск: tools/proxy.sh [user@host]          поднять и показать ссылку
#         tools/proxy.sh [user@host] down     остановить
#         tools/proxy.sh [user@host] rotate   выпустить новый секрет (старые ссылки умрут)

set -euo pipefail

host="${1:-${BARDAK_HOST:-root@93.171.232.185}}"
action="${2:-up}"
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(dirname "$here")"
image="nineseconds/mtg:2.1.7"
ssh_=(ssh -o BatchMode=yes "$host")
compose="docker compose -f deploy/compose.prod.yaml --env-file deploy/.env --profile proxy"

if ! "${ssh_[@]}" 'test -s ~/bardak/deploy/.env'; then
    echo "❌ на сервере нет deploy/.env" >&2
    exit 1
fi

if [[ "$action" == down ]]; then
    "${ssh_[@]}" "cd ~/bardak && $compose stop mtproto && $compose rm -f mtproto"
    exit 0
fi

if [[ "$action" == rotate ]]; then
    "${ssh_[@]}" "cd ~/bardak && cp deploy/.env deploy/.env.bak && sed -i '/^MTG_SECRET=/d' deploy/.env"
fi

echo "▸ отправляю compose"
scp -q -o BatchMode=yes "$root/deploy/compose.prod.yaml" "$host:~/bardak/deploy/"

"${ssh_[@]}" "set -euo pipefail; cd ~/bardak
    if ! grep -qE '^MTG_SECRET=.+' deploy/.env; then
        domain=\$(sed -n 's/^MTG_DOMAIN=//p' deploy/.env)
        domain=\${domain:-\$(sed -n 's/^BARDAK_DOMAIN=//p' deploy/.env)}
        secret=\$(docker run --rm $image generate-secret --hex \"\$domain\")
        cp deploy/.env deploy/.env.bak
        sed -i '/^MTG_SECRET=/d' deploy/.env
        printf '\nMTG_SECRET=%s\n' \"\$secret\" >> deploy/.env
        echo \"▸ выпущен секрет на \$domain\"
    fi
    grep -qE '^MTG_PORT=' deploy/.env || echo 'MTG_PORT=8443' >> deploy/.env
    $compose up -d mtproto"

sleep 2
env="$("${ssh_[@]}" 'grep -E "^MTG_" ~/bardak/deploy/.env')"
secret="$(sed -n 's/^MTG_SECRET=//p' <<<"$env")"
port="$(sed -n 's/^MTG_PORT=//p' <<<"$env")"
public="$(sed -n 's/^MTG_PUBLIC_HOST=//p' <<<"$env")"
public="${public:-${host#*@}}"

if ! "${ssh_[@]}" "docker ps --filter name=bardak-mtproto-1 --filter status=running -q" | grep -q .; then
    echo "❌ контейнер не запустился:" >&2
    "${ssh_[@]}" "cd ~/bardak && $compose logs --tail 20 mtproto" >&2
    exit 1
fi
echo "✅ прокси работает"
echo "tg://proxy?server=$public&port=$port&secret=$secret"
echo "https://t.me/proxy?server=$public&port=$port&secret=$secret"
