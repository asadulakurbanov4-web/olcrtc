#!/usr/bin/env bash
# roomman.sh — простой менеджер комнат olcRTC (серверная часть)
# Работает с уже собранным бинарником ./olcrtc в ~/olcrtc
# Комнаты: каждая = отдельный контейнер podman + своя папка ~/.olcrtc-rooms/<имя>
set -e

BIN_DIR="$HOME/olcrtc"                       # папка с бинарником olcrtc
ROOMS_DIR="$HOME/.olcrtc-rooms"              # папка со всеми комнатами
IMAGE="docker.io/library/golang:1.26-alpine3.22"
DEFAULT_HOST="https://meet.mamba.group"      # хост по умолчанию (egovm исключён из-за TLS-проблем на Android)

red()   { printf '\033[31m%s\033[0m\n' "$*"; }
green() { printf '\033[32m%s\033[0m\n' "$*"; }
yellow(){ printf '\033[33m%s\033[0m\n' "$*"; }

need_bin() {
    if [ ! -x "$BIN_DIR/olcrtc" ]; then
        red "Не найден бинарник $BIN_DIR/olcrtc. Сначала соберите его (mage build или install.sh)."
        exit 1
    fi
}

cname() { echo "olcrtc-room-$1"; }

cmd_new() {
    need_bin
    echo ""
    echo "=== Новая комната ==="
    read -p "Имя комнаты (латиница/цифры, например work, home2): " NAME
    if [ -z "$NAME" ] || [[ "$NAME" =~ [^a-zA-Z0-9_-] ]]; then
        red "Недопустимое имя. Разрешены латинские буквы, цифры, дефис и подчёркивание."
        exit 1
    fi
    DIR="$ROOMS_DIR/$NAME"
    if [ -d "$DIR" ]; then
        red "Комната '$NAME' уже существует. Удалите её командой: $0 del $NAME"
        exit 1
    fi

    echo "Хосты Jitsi (откройте в браузере и проверьте, что сайт работает!):"
    HOSTS=()
    while IFS= read -r h; do [ -n "$h" ] && HOSTS+=("$h"); done < <(curl -fsSL "https://raw.githubusercontent.com/openlibrecommunity/olcrtc/master/docs/jitsi.instances.yaml" 2>/dev/null | sed -n 's/^  - //p')
    i=1
    for h in "${HOSTS[@]}"; do echo "  $i) https://$h/"; i=$((i+1)); done
    echo "  $i) Другой (вручную)"
    read -p "Выберите номер [1-$i, по умолчанию 1]: " HC
    HC=${HC:-1}
    if [ "$HC" = "$i" ]; then
        read -p "Введите URL Jitsi: " BASE
        BASE="${BASE%/}"
    elif [[ "$HC" =~ ^[0-9]+$ ]] && [ "$HC" -ge 1 ] && [ "$HC" -lt "$i" ]; then
        BASE="https://${HOSTS[$((HC-1))]}"
    else
        BASE="$DEFAULT_HOST"
    fi
    if [ -z "$BASE" ]; then red "Пустой URL хоста"; exit 1; fi

    ROOM_URL="$BASE/olcrtc-$NAME-$(tr -dc 'a-z0-9' </dev/urandom | head -c 8)"
    KEY=$(openssl rand -hex 32)

    mkdir -p "$DIR"
    cat > "$DIR/config.yaml" <<EOF
mode: srv
auth:
  provider: "jitsi"
room:
  id: "$ROOM_URL"
crypto:
  key: "$KEY"
net:
  transport: "datachannel"
  dns: "8.8.8.8:53"
EOF
    chmod 700 "$DIR"

    cp "$BIN_DIR/olcrtc" "$DIR/olcrtc"
    podman run -d \
        --name "$(cname "$NAME")" \
        --network host \
        --restart unless-stopped \
        -v "$DIR":/app:Z \
        -w /app \
        "$IMAGE" \
        sh -c "./olcrtc config.yaml" >/dev/null

    sleep 2
    if podman ps --filter name="$(cname "$NAME")" --format '{{.Names}}' | grep -q .; then
        green "[+] Комната '$NAME' запущена!"
    else
        red "[X] Контейнер не запустился. Логи: podman logs $(cname "$NAME")"
        exit 1
    fi

    URI="olcrtc://jitsi?datachannel@$ROOM_URL#$KEY\$room:$NAME"
    echo ""
    echo "Provider:  jitsi"
    echo "Transport: datachannel"
    echo "Room URL:  $ROOM_URL"
    echo "Key:       $KEY"
    echo ""
    yellow "Скопируйте эту строку целиком в olcbox на телефоне:"
    echo "$URI" > "$DIR/uri.txt"
    echo "  (она же сохранена в $DIR/uri.txt)"
    echo ""
    echo "$URI"
}

cmd_list() {
    echo "=== Комнаты ==="
    found=0
    for d in "$ROOMS_DIR"/*/; do
        [ -d "$d" ] || continue
        found=1
        NAME=$(basename "$d")
        C=$(cname "$NAME")
        STATUS=$(podman ps -a --filter name="$C" --format '{{.Status}}' 2>/dev/null | head -1)
        [ -z "$STATUS" ] && STATUS="(нет контейнера)"
        ROOM=$(sed -n 's/^  id: "\(.*\)"/\1/p' "$d/config.yaml" 2>/dev/null)
        printf "%-15s %-25s %s\n" "$NAME" "$STATUS" "$ROOM"
    done
    [ "$found" = "0" ] && yellow "Комнат пока нет. Создайте: $0 new"
    echo ""
    echo "URI комнаты: $0 uri <имя>"
}

cmd_uri() {
    NAME="$1"
    [ -z "$NAME" ] && { red "Использование: $0 uri <имя_комнаты>"; exit 1; }
    F="$ROOMS_DIR/$NAME/uri.txt"
    [ -f "$F" ] || { red "Комната '$NAME' не найдена (нет $F). Посмотрите список: $0 list"; exit 1; }
    cat "$F"
}

cmd_stop() {
    NAME="$1"
    [ -z "$NAME" ] && { red "Использование: $0 stop <имя_комнаты>"; exit 1; }
    podman stop "$(cname "$NAME")" && green "Остановлена: $NAME"
}

cmd_start() {
    NAME="$1"
    [ -z "$NAME" ] && { red "Использование: $0 start <имя_комнаты>"; exit 1; }
    podman start "$(cname "$NAME")" && green "Запущена: $NAME"
}

cmd_logs() {
    NAME="$1"
    [ -z "$NAME" ] && { red "Использование: $0 logs <имя_комнаты>"; exit 1; }
    podman logs -f --tail 50 "$(cname "$NAME")"
}

cmd_del() {
    NAME="$1"
    [ -z "$NAME" ] && { red "Использование: $0 del <имя_комнаты>"; exit 1; }
    yellow "Удалить комнату '$NAME' вместе с ключом и конфигом? Это нельзя отменить."
    read -p "Ответьте yes для подтверждения: " OK
    [ "$OK" = "yes" ] || { echo "Отменено."; exit 0; }
    podman rm -f "$(cname "$NAME")" 2>/dev/null || true
    rm -rf "$ROOMS_DIR/$NAME"
    green "Удалена: $NAME"
}

case "$1" in
    new)   cmd_new ;;
    list)  cmd_list ;;
    uri)   cmd_uri "$2" ;;
    start) cmd_start "$2" ;;
    stop)  cmd_stop "$2" ;;
    logs)  cmd_logs "$2" ;;
    del)   cmd_del "$2" ;;
    *)
        cat <<EOF
Менеджер комнат olcRTC

Использование: $0 <команда> [имя_комнаты]

Команды:
  new            создать и запустить новую комнату (ключ генерируется сам)
  list           список комнат и их статус
  uri <имя>      показать olcrtc:// строку для телефона
  start <имя>    запустить остановленную комнату
  stop <имя>     остановить комнату
  logs <имя>     смотреть логи комнаты (выход — Ctrl+C)
  del <имя>      удалить комнату навсегда
EOF
        ;;
esac
