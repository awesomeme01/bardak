#!/usr/bin/env bash
#
# VPN «только для Telegram» рядом с игрой (WireGuard, split-tunnel по подсетям Telegram).
#
# ⭐ Список клиентов и подсети живут в deploy/.env на сервере (WG_PEERS, WG_ALLOWEDIPS),
# ключи — в томе wg-config. Здесь только команды.
# ⚠️ .env не перезаписывается целиком: меняется ровно одна строка, копия — в .env.bak.
# ⚠️ add/remove/refresh-ranges пересоздают контейнер: туннель у всех рвётся на пару секунд.
#
# Запуск: tools/vpn.sh add <имя>        завести клиента, показать QR, сохранить .conf
#         tools/vpn.sh show <имя>       снова показать QR и .conf
#         tools/vpn.sh remove <имя>     отозвать доступ
#         tools/vpn.sh list             кто подключён, трафик
#         tools/vpn.sh up               обновить конфигурацию и перезапустить
#         tools/vpn.sh refresh-ranges   перекачать подсети Telegram (конфиги раздать заново)
#         tools/vpn.sh down             остановить
# Сервер: BARDAK_HOST=user@host (по умолчанию прод).

set -euo pipefail

host="${BARDAK_HOST:-root@93.171.232.185}"
action="${1:-list}"
name="${2:-}"
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(dirname "$here")"
cidr_file="$root/deploy/wireguard/telegram-cidr.txt"
local_dir="$HOME/bardak-vpn"
container=bardak-wireguard-1
ssh_=(ssh -o BatchMode=yes "$host")
compose="docker compose -f deploy/compose.prod.yaml --env-file deploy/.env --profile vpn"

if ! "${ssh_[@]}" 'test -s ~/bardak/deploy/.env'; then
    echo "❌ на сервере нет deploy/.env" >&2
    exit 1
fi

env_get() {
    "${ssh_[@]}" "sed -n 's/^$1=//p' ~/bardak/deploy/.env" | tail -1
}

# ⚠️ Меняем одну строку через временный файл и проверяем, что остальное на месте:
# испорченный .env — это потерянный пароль базы (см. deploy/README.md).
env_set() {
    "${ssh_[@]}" "set -e; cd ~/bardak/deploy
        cp .env .env.bak
        grep -v '^$1=' .env > .env.tmp || true
        printf '%s=%s\n' '$1' '$2' >> .env.tmp
        grep -q '^POSTGRES_PASSWORD=' .env.tmp
        mv .env.tmp .env"
}

need_name() {
    if [[ ! "$name" =~ ^[A-Za-z0-9]+$ ]]; then
        echo "❌ имя клиента — только латинские буквы и цифры (так требует образ): '$name'" >&2
        exit 2
    fi
}

up() {
    local peers
    peers="$(env_get WG_PEERS)"
    if [[ -z "$peers" ]]; then
        echo "❌ клиентов нет — начни с tools/vpn.sh add <имя>" >&2
        exit 2
    fi
    echo "▸ отправляю compose и шаблон клиента"
    "${ssh_[@]}" 'mkdir -p ~/bardak/deploy/wireguard'
    scp -q -o BatchMode=yes "$root/deploy/compose.prod.yaml" "$host:~/bardak/deploy/"
    scp -q -o BatchMode=yes "$root/deploy/wireguard/peer.conf" "$host:~/bardak/deploy/wireguard/"

    local ranges
    ranges="$(grep -vE '^\s*(#|$)' "$cidr_file" | paste -sd, -)"
    [[ "$(env_get WG_ALLOWEDIPS)" == "$ranges" ]] || env_set WG_ALLOWEDIPS "$ranges"
    [[ -n "$(env_get WG_PUBLIC_HOST)" ]] || env_set WG_PUBLIC_HOST "${host#*@}"

    echo "▸ поднимаю (клиенты: $peers)"
    "${ssh_[@]}" "cd ~/bardak && $compose up -d --force-recreate wireguard"

    for _ in $(seq 1 15); do
        if "${ssh_[@]}" "docker exec $container wg show wg0 >/dev/null 2>&1"; then
            echo "✅ VPN работает"
            return 0
        fi
        sleep 2
    done
    echo "❌ интерфейс wg0 не поднялся:" >&2
    "${ssh_[@]}" "docker logs --tail 30 $container" >&2
    exit 1
}

show() {
    need_name
    local conf="/config/peer_$name/peer_$name.conf"
    if ! "${ssh_[@]}" "docker exec $container test -f $conf"; then
        echo "❌ клиента '$name' нет" >&2
        exit 1
    fi
    mkdir -p "$local_dir" && chmod 700 "$local_dir"
    "${ssh_[@]}" "docker exec $container cat $conf" > "$local_dir/$name.conf"
    chmod 600 "$local_dir/$name.conf"
    echo
    echo "📱 телефон: приложение WireGuard → «+» → «Сканировать QR-код»"
    "${ssh_[@]}" "docker exec $container /app/show-peer $name"
    echo "💻 компьютер: WireGuard → «Импорт туннеля из файла» → $local_dir/$name.conf"
    echo "⚠️ конфиг — это доступ: передавать лично, одно устройство — один конфиг"
}

case "$action" in
    add)
        need_name
        peers="$(env_get WG_PEERS)"
        if [[ ",$peers," == *",$name,"* ]]; then
            echo "▸ '$name' уже есть — показываю"
        else
            env_set WG_PEERS "${peers:+$peers,}$name"
            up
        fi
        show
        ;;
    show)
        show
        ;;
    remove)
        need_name
        peers="$(env_get WG_PEERS)"
        rest="$(tr ',' '\n' <<<"$peers" | grep -vx "$name" | paste -sd, - || true)"
        if [[ "$rest" == "$peers" ]]; then
            echo "❌ клиента '$name' нет" >&2
            exit 1
        fi
        env_set WG_PEERS "$rest"
        # Папку с ключами удаляем, иначе при повторном add вернулся бы старый ключ.
        "${ssh_[@]}" "docker exec $container rm -rf /config/peer_$name"
        rm -f "$local_dir/$name.conf"
        if [[ -n "$rest" ]]; then
            up
        else
            "${ssh_[@]}" "cd ~/bardak && $compose stop wireguard"
        fi
        echo "✅ '$name' отозван"
        ;;
    list)
        # wg show знает только публичные ключи — подставляем имена из папок клиентов.
        "${ssh_[@]}" "docker exec $container sh -c '
            for d in /config/peer_*; do
                n=\${d#/config/peer_}
                echo \"\$(cat \$d/publickey-peer_\$n) \$n\"
            done > /tmp/names
            wg show wg0 dump | tail -n +2 | while read -r key psk endpoint ips hs rx tx ka; do
                n=\$(grep \"^\$key \" /tmp/names | cut -d\" \" -f2)
                if [ \"\$hs\" = 0 ]; then seen=\"ни разу\"; else seen=\"\$(( \$(date +%s) - hs )) с назад\"; fi
                printf \"%-16s рукопожатие: %-14s ↓ %s Б  ↑ %s Б\n\" \"\${n:-?}\" \"\$seen\" \"\$tx\" \"\$rx\"
            done'"
        ;;
    up)
        up
        ;;
    refresh-ranges)
        new="$("${ssh_[@]}" 'curl -fsS -m10 https://core.telegram.org/resources/cidr.txt')"
        if diff <(cat "$cidr_file") <(echo "$new"); then
            echo "✅ подсети не менялись"
            exit 0
        fi
        echo "$new" > "$cidr_file"
        up
        echo "⚠️ подсети изменились: конфиги клиентов перевыпущены — раздай заново (tools/vpn.sh show <имя>)"
        ;;
    down)
        "${ssh_[@]}" "cd ~/bardak && $compose stop wireguard"
        ;;
    *)
        sed -n '10,17p' "$0"
        exit 2
        ;;
esac
