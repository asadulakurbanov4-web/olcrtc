════════════════════════════════════════════════════════════════════════════════
<!-- ФАЙЛ 8/10: 07_Инфраструктура.md -->
════════════════════════════════════════════════════════════════════════════════

# OlcPanel — Раздел 7: Инфраструктура
## Nginx · systemd · Let's Encrypt · Деплой · Бэкап · Мониторинг

> **Версия:** 2.0 | **Статус:** 🟡 В разработке
> **ОС:** Ubuntu 22.04 LTS
> **Пользователь сервиса:** `olcpanel` (не root)
>
> ⚠️ **v2.0 инфра-сдвиги (раздел 7.10):** добавлены операционные процедуры **`authz`/`Gate` с last-known-good** (5.2): рестарт `olcrtc` только после валидации `authz.json` (cold-start = `closed`), конфиг `authz.fail_mode: lkg` в `server.yaml`, доставка среза (rsync+trigger с атомарным `rename`), health-эндпоинт. Бэкап теперь включает `authz.json`. iptables-учёт удалён (Решение 5) — соответствующие sudoers больше не нужны.
> **Корень проекта:** `/opt/olcpanel`
> **База данных:** `/opt/olcpanel/data/olcpanel.db`
> **Зависимости:** [[00_Введение]], [[03_Архитектура_сервисов]]

---

## 7.1 Общая схема инфраструктуры

```
                              INTERNET
                                 │ :443 (HTTPS)
                                 │ :80  (HTTP → redirect)
                        ┌────────▼────────┐
                        │     Nginx       │
                        │  /etc/nginx/    │
                        │  conf.d/        │
                        │  olcpanel.conf  │
                        │                 │
                        │ • TLS termination│
                        │ • Let's Encrypt │
                        │ • Static files  │
                        │ • Rate limiting │
                        │ • Security hdrs │
                        │ • proxy_pass    │
                        └────────┬────────┘
                                 │ :8000 (localhost only)
                        ┌────────▼────────┐
                        │  OlcPanel       │
                        │  (FastAPI +     │
                        │   Uvicorn)      │
                        │                 │
                        │ systemd:        │
                        │ olcpanel.service│
                        │ user: olcpanel  │
                        │ /opt/olcpanel/  │
                        └────────┬────────┘
                                 │ subprocess (systemctl, iptables)
                                 │ файловая система (server.yaml, logs)
                        ┌────────▼────────┐
                        │  OlcRTC Server  │
                        │                 │
                        │ systemd:        │
                        │ olcrtc-server.  │
                        │ service         │
                        │ /root/olcrtc/   │
                        └─────────────────┘

### 7.2 Горизонтальное масштабирование (много дешёвых VPS, central panel)

**Стратегия (практический взгляд):**
- Аренда множества дешёвых VPS.
- 50–100 клиентов на сервер как целевой диапазон (для снижения нагрузки на отдельную комнату и ограничения blast radius).
- **Долгосрочно — одна центральная панель**. На 1–3 серверах допустимо временное использование одной панели на главном сервере + ручное/скриптовое управление остальными. Модель данных и сервисы должны с самого начала поддерживать `server_id`, чтобы миграция не превратилась в рефакторинг БД.
- authz синхронизация — одна из самых сложных частей. См. детальное сравнение вариантов (rsync+trigger, signed HTTP push, онлайн-проверка) и рекомендации по этапам в olcrtc-panel-architecture.md. Текущий локальный writer перестаёт работать, как только панель и olcrtc оказываются на разных машинах.
- Каждый сервер продолжает иметь свой `server.yaml` и свой источник allowlist (authz.json или его аналог). Центральная панель отвечает за доставку актуального содержимого.
- Мониторинг и наблюдаемость authz-операций должны быть на порядок лучше, чем в single-server модели (метрики успешности push по каждому серверу, время применения, расхождения allowlist).

**Схема (центральная панель + N серверов):**
```
Internet (клиенты olcbox)
          │
   Central Panel (один VPS или dedicated)
   (Nginx + FastAPI + SQLite + ServerRegistry + AuthzCoordinator)
          │ health / RPC / rsync authz.json / deploy
   ┌──────┴──────┬──────────┬──────────┐
   VPS1 (olcrtc) VPS2 ...   VPSn
   (50-100 клиентов, свой server.yaml, свой authz.json, Gate Phase 1)
```

**На этапе MVP (один сервер):** инфраструктура остаётся как в 7.1. Переход к horizontal — добавление таблицы `servers`, поля `server_id` в `user_devices`/`subscriptions`, health endpoints и fleet-скриптов (без ломки текущего running /opt/olcrtc-panel на 212.192.253.241).

См. также olcrtc-panel-architecture.md (разделы Scaling Strategy + Phased Roadmap) и OlcPanel_03 (эволюция сервисов).

Дополнительные процессы:
  ┌─────────────────────────────────────────────────────────┐
  │  cron / systemd.timer                                   │
  │    • olcpanel-backup.timer   — ежедневный бэкап SQLite  │
  │    • certbot.timer           — автообновление TLS       │
  └─────────────────────────────────────────────────────────┘
```

---

## 7.2 Структура директорий на сервере

```
/opt/olcpanel/                ← основная директория проекта
├── .env                      ← секреты (chmod 600, владелец olcpanel)
├── .env.example
├── requirements.txt
├── alembic.ini
├── app/                      ← Python-пакет (исходный код)
│   ├── main.py
│   ├── config.py
│   ├── database.py
│   ├── models/
│   ├── schemas/
│   ├── routers/
│   ├── services/
│   ├── repositories/
│   ├── middleware/
│   └── core/
├── alembic/                  ← миграции БД
│   └── versions/
├── static/                   ← Frontend (Nginx отдаёт напрямую)
│   ├── index.html
│   ├── login.html
│   ├── css/
│   └── js/
├── data/                     ← runtime данные (chmod 700)
│   └── olcpanel.db           ← SQLite database
├── logs/                     ← логи приложения (chmod 755)
│   ├── app.log               ← основной лог
│   └── audit.log             ← аудит (только append)
├── backups/                  ← автобэкапы SQLite
│   ├── olcpanel-20260612.db.gz
│   └── olcpanel-20260611.db.gz
├── scripts/
│   ├── install.sh            ← установка с нуля
│   ├── backup.sh             ← скрипт бэкапа
│   ├── create_admin.py       ← создание первого администратора
│   └── check_health.sh       ← проверка всех компонентов
├── systemd/
│   ├── olcpanel.service      ← шаблон юнита (копируется при установке)
│   └── olcpanel-backup.timer ← шаблон таймера
└── venv/                     ← Python virtualenv (создаётся при установке)

/root/olcrtc/                 ← существующий OlcRTC-сервер (не трогаем)
├── build/
│   └── olcrtc-linux-amd64
├── server.yaml
└── docs/
    └── good-carriers.md

/etc/nginx/conf.d/
└── olcpanel.conf             ← Nginx конфиг панели

/etc/systemd/system/
├── olcpanel.service          ← systemd юнит панели
├── olcrtc-server.service     ← systemd юнит OlcRTC (уже существует)
└── olcpanel-backup.timer     ← таймер бэкапа

/etc/sudoers.d/
└── olcpanel                  ← sudo правила для olcpanel user

/var/log/nginx/
├── olcpanel-access.log       ← access лог Nginx
└── olcpanel-error.log        ← error лог Nginx
```

---

## 7.3 Nginx — полная конфигурация

### 7.3.1 Конфиг `/etc/nginx/conf.d/olcpanel.conf`

```nginx
# /etc/nginx/conf.d/olcpanel.conf
# OlcPanel — Nginx конфигурация
# Ubuntu 22.04, Nginx 1.18+

# === Зона rate limiting (объявляется на уровне http, но у нас в conf.d) ===
# Если несколько сайтов, вынести в /etc/nginx/nginx.conf → http {}
limit_req_zone $binary_remote_addr zone=login:10m rate=5r/m;
limit_req_zone $binary_remote_addr zone=api:10m   rate=60r/m;

# === HTTP → HTTPS redirect ===
server {
    listen 80;
    listen [::]:80;
    server_name your-panel-domain.com;     # ← заменить на реальный домен

    # Let's Encrypt challenge (certbot webroot)
    location /.well-known/acme-challenge/ {
        root /var/www/certbot;
        allow all;
    }

    # Всё остальное — редирект на HTTPS
    location / {
        return 301 https://$host$request_uri;
    }
}

# === HTTPS server ===
server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name your-panel-domain.com;     # ← заменить на реальный домен

    # === TLS ===
    ssl_certificate     /etc/letsencrypt/live/your-panel-domain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/your-panel-domain.com/privkey.pem;
    ssl_trusted_certificate /etc/letsencrypt/live/your-panel-domain.com/chain.pem;

    # Современные настройки TLS (Mozilla Intermediate)
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-ECDSA-CHACHA20-POLY1305:ECDHE-RSA-CHACHA20-POLY1305:DHE-RSA-AES128-GCM-SHA256;
    ssl_prefer_server_ciphers off;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 1d;
    ssl_session_tickets off;
    ssl_stapling on;
    ssl_stapling_verify on;
    resolver 8.8.8.8 1.1.1.1 valid=300s;
    resolver_timeout 5s;

    # === Security Headers ===
    add_header Strict-Transport-Security "max-age=63072000; includeSubDomains; preload" always;
    add_header X-Frame-Options "DENY" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;
    add_header Permissions-Policy "camera=(), microphone=(), geolocation=()" always;

    # CSP (Content Security Policy)
    # Адаптировать при добавлении CDN с SRI
    add_header Content-Security-Policy "
        default-src 'self';
        script-src 'self' https://cdn.jsdelivr.net;
        style-src 'self' 'unsafe-inline';
        img-src 'self' data:;
        font-src 'self';
        connect-src 'self';
        frame-ancestors 'none';
        base-uri 'self';
        form-action 'self';
    " always;

    # === Логи ===
    access_log /var/log/nginx/olcpanel-access.log combined;
    error_log  /var/log/nginx/olcpanel-error.log warn;

    # === Основные настройки ===
    client_max_body_size 1m;          # максимальный размер запроса
    keepalive_timeout 65;
    gzip on;
    gzip_vary on;
    gzip_min_length 1024;
    gzip_types text/plain text/css text/javascript application/json application/javascript;

    # === Статические файлы (Frontend) ===
    # Nginx отдаёт напрямую, не через FastAPI
    root /opt/olcpanel/static;

    location = / {
        # Корень: если авторизован → index.html, иначе → login.html
        # Проверку авторизации делает JS (смотрит на cookie)
        try_files /index.html =404;
    }

    location = /login {
        try_files /login.html =404;
    }

    location /css/ {
        expires 7d;
        add_header Cache-Control "public, immutable";
    }

    location /js/ {
        expires 7d;
        add_header Cache-Control "public, immutable";
    }

    # Не кэшируем index.html и login.html (могут измениться)
    location ~* \.(html)$ {
        expires -1;
        add_header Cache-Control "no-cache, no-store, must-revalidate";
    }

    # === API → FastAPI (rate limited) ===

    # Login endpoint — строгий rate limit
    location = /api/auth/login {
        limit_req zone=login burst=3 nodelay;
        limit_req_status 429;
        proxy_pass http://127.0.0.1:8000;
        include /etc/nginx/conf.d/proxy_params.conf;
    }

    # Health endpoint — без rate limit, без auth
    location = /api/health {
        proxy_pass http://127.0.0.1:8000;
        include /etc/nginx/conf.d/proxy_params.conf;
    }

    # Swagger UI — только с localhost (защита документации)
    location = /api/docs {
        allow 127.0.0.1;
        deny all;
        proxy_pass http://127.0.0.1:8000;
        include /etc/nginx/conf.d/proxy_params.conf;
    }

    location = /api/openapi.json {
        allow 127.0.0.1;
        deny all;
        proxy_pass http://127.0.0.1:8000;
        include /etc/nginx/conf.d/proxy_params.conf;
    }

    # Все остальные API-запросы — умеренный rate limit
    location /api/ {
        limit_req zone=api burst=20 nodelay;
        limit_req_status 429;
        proxy_pass http://127.0.0.1:8000;
        include /etc/nginx/conf.d/proxy_params.conf;
    }

    # Запрет прямого доступа к данным
    location /data/ { deny all; return 404; }
    location /logs/  { deny all; return 404; }
    location /venv/  { deny all; return 404; }
    location ~ /\.   { deny all; return 404; }  # скрытые файлы (.env и т.д.)
}
```

### 7.3.2 Параметры proxy `/etc/nginx/conf.d/proxy_params.conf`

```nginx
# /etc/nginx/conf.d/proxy_params.conf
# Общие параметры для proxy_pass к FastAPI

proxy_http_version 1.1;
proxy_set_header Host              $host;
proxy_set_header X-Real-IP         $remote_addr;
proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
proxy_set_header X-Forwarded-Proto $scheme;
proxy_set_header Connection        "";

proxy_connect_timeout   10s;
proxy_send_timeout      30s;
proxy_read_timeout      30s;
proxy_buffering         on;
proxy_buffer_size       4k;
proxy_buffers           8 4k;

# Передаём реальный IP для rate limiting в FastAPI
proxy_set_header X-Client-IP $remote_addr;
```

---

## 7.4 systemd — юниты

### 7.4.1 `olcpanel.service`

```ini
# /etc/systemd/system/olcpanel.service
# OlcPanel FastAPI сервис

[Unit]
Description=OlcPanel — VPN Keys Management Panel
Documentation=https://github.com/your-org/olcpanel
After=network.target
Wants=network.target

[Service]
# === Пользователь и группа ===
User=olcpanel
Group=olcpanel
WorkingDirectory=/opt/olcpanel

# === Переменные окружения ===
EnvironmentFile=/opt/olcpanel/.env

# === Запуск ===
ExecStart=/opt/olcpanel/venv/bin/uvicorn app.main:app \
    --host 127.0.0.1 \
    --port 8000 \
    --workers 1 \
    --log-level info \
    --access-log \
    --no-use-colors

# Предзапуск: проверяем что .env существует
ExecStartPre=/bin/test -f /opt/olcpanel/.env

# === Перезапуск ===
Restart=always
RestartSec=5s
StartLimitIntervalSec=60
StartLimitBurst=5

# === Логирование ===
StandardOutput=journal
StandardError=journal
SyslogIdentifier=olcpanel

# === Безопасность ===
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=read-only
ReadWritePaths=/opt/olcpanel/data
ReadWritePaths=/opt/olcpanel/logs
ReadWritePaths=/opt/olcpanel/backups
ReadOnlyPaths=/root/olcrtc/docs
ReadOnlyPaths=/root/olcrtc/server.yaml
ReadOnlyPaths=/tmp

# Сетевые права
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
IPAddressAllow=localhost
IPAddressDeny=any

# Системные вызовы (белый список)
SystemCallFilter=@system-service
SystemCallErrorNumber=EPERM

# Ресурсы
MemoryMax=512M
CPUQuota=80%
TasksMax=64

# Возможности (только необходимые для iptables через sudo)
AmbientCapabilities=
CapabilityBoundingSet=

# Таймаут при остановке
TimeoutStopSec=30

[Install]
WantedBy=multi-user.target
```

### 7.4.2 `olcrtc-server.service` (создаём если ещё нет)

```ini
# /etc/systemd/system/olcrtc-server.service
# OlcRTC Server сервис

[Unit]
Description=OlcRTC Server — WebRTC VPN Tunnel
After=network.target
Wants=network.target

[Service]
User=root
WorkingDirectory=/root/olcrtc

ExecStart=/root/olcrtc/build/olcrtc-linux-amd64 \
    --config /root/olcrtc/server.yaml

Restart=always
RestartSec=10s
StartLimitIntervalSec=120
StartLimitBurst=3

StandardOutput=journal
StandardError=journal
SyslogIdentifier=olcrtc-server

# Таймаут
TimeoutStopSec=15

[Install]
WantedBy=multi-user.target
```

### 7.4.3 `olcpanel-backup.service` и `olcpanel-backup.timer`

```ini
# /etc/systemd/system/olcpanel-backup.service
# Разовый сервис для бэкапа (запускается таймером)

[Unit]
Description=OlcPanel SQLite Backup
After=olcpanel.service

[Service]
Type=oneshot
User=olcpanel
ExecStart=/opt/olcpanel/scripts/backup.sh
StandardOutput=journal
StandardError=journal
SyslogIdentifier=olcpanel-backup
```

```ini
# /etc/systemd/system/olcpanel-backup.timer
# Запускает бэкап ежедневно в 03:00 UTC

[Unit]
Description=Daily OlcPanel SQLite Backup
Requires=olcpanel-backup.service

[Timer]
OnCalendar=*-*-* 03:00:00 UTC
Persistent=true             # запустить если пропустили (был выключен)
RandomizedDelaySec=300      # случайная задержка 0-5 мин (не нагружать в одно время)

[Install]
WantedBy=timers.target
```

---

## 7.5 Let's Encrypt / Certbot

### 7.5.1 Установка и получение сертификата

```bash
# Установка Certbot + Nginx плагин
apt install -y certbot python3-certbot-nginx

# Создать директорию для webroot challenge
mkdir -p /var/www/certbot

# Получить сертификат
# Метод: webroot (Nginx уже запущен и отдаёт /.well-known/)
certbot certonly \
    --webroot \
    --webroot-path /var/www/certbot \
    --email your@email.com \
    --agree-tos \
    --no-eff-email \
    -d your-panel-domain.com

# Сертификаты будут в:
# /etc/letsencrypt/live/your-panel-domain.com/
#   fullchain.pem  ← ssl_certificate
#   privkey.pem    ← ssl_certificate_key
#   chain.pem      ← ssl_trusted_certificate (для OCSP stapling)
```

### 7.5.2 Автообновление

Certbot при установке создаёт `/etc/cron.d/certbot` или `certbot.timer` автоматически.

Проверить:
```bash
systemctl list-timers | grep certbot
# certbot.timer  —  каждые 12 часов

# Тестовый прогон (без реального обновления)
certbot renew --dry-run
```

### 7.5.3 Hook для перезапуска Nginx после обновления

```bash
# /etc/letsencrypt/renewal-hooks/deploy/reload-nginx.sh

#!/bin/bash
systemctl reload nginx
echo "Nginx reloaded after certificate renewal at $(date)" >> /var/log/certbot-nginx-reload.log
```

```bash
chmod +x /etc/letsencrypt/renewal-hooks/deploy/reload-nginx.sh
```

---

## 7.6 Скрипт бэкапа

### 7.6.1 `/opt/olcpanel/scripts/backup.sh`

```bash
#!/bin/bash
# OlcPanel SQLite Backup Script
# Запускается ежедневно в 03:00 UTC через systemd timer
# Хранит 7 дней резервных копий

set -euo pipefail

# === Конфигурация ===
DB_PATH="/opt/olcpanel/data/olcpanel.db"
BACKUP_DIR="/opt/olcpanel/backups"
KEEP_DAYS=7
DATE=$(date +%Y%m%d)
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="${BACKUP_DIR}/olcpanel-${DATE}.db.gz"

# === Логирование ===
log() { echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] $*"; }

log "Starting OlcPanel backup..."

# === Проверки ===
if [ ! -f "$DB_PATH" ]; then
    log "ERROR: Database file not found: $DB_PATH"
    exit 1
fi

mkdir -p "$BACKUP_DIR"
chmod 700 "$BACKUP_DIR"

# === Бэкап ===
# Используем sqlite3 .backup для горячего бэкапа (WAL mode, без блокировок)
# Это безопаснее чем простое cp при работающем сервисе

TEMP_BACKUP="/tmp/olcpanel-backup-${TIMESTAMP}.db"

sqlite3 "$DB_PATH" ".backup '${TEMP_BACKUP}'"

if [ ! -f "$TEMP_BACKUP" ]; then
    log "ERROR: Backup creation failed"
    exit 1
fi

# Сжимаем
gzip -c "$TEMP_BACKUP" > "$BACKUP_FILE"
rm -f "$TEMP_BACKUP"

# Права на файл
chmod 600 "$BACKUP_FILE"

# Проверяем целостность бэкапа
BACKUP_SIZE=$(stat -c%s "$BACKUP_FILE")
if [ "$BACKUP_SIZE" -lt 1024 ]; then
    log "WARNING: Backup file seems too small (${BACKUP_SIZE} bytes)"
fi

log "Backup created: ${BACKUP_FILE} ($(du -h "$BACKUP_FILE" | cut -f1))"

# === Ротация — удаляем файлы старше 7 дней ===
DELETED=$(find "$BACKUP_DIR" -name "olcpanel-*.db.gz" -mtime +${KEEP_DAYS} -delete -print | wc -l)
if [ "$DELETED" -gt 0 ]; then
    log "Deleted ${DELETED} old backup(s)"
fi

# === Список текущих бэкапов ===
BACKUP_COUNT=$(find "$BACKUP_DIR" -name "olcpanel-*.db.gz" | wc -l)
log "Current backups: ${BACKUP_COUNT} files"

log "Backup completed successfully"
```

```bash
chmod +x /opt/olcpanel/scripts/backup.sh
```

---

## 7.7 sudoers для пользователя olcpanel

```bash
# /etc/sudoers.d/olcpanel
# Минимальные права для пользователя olcpanel

# Управление сервисами
olcpanel ALL=(ALL) NOPASSWD: /bin/systemctl is-active olcrtc-server
olcpanel ALL=(ALL) NOPASSWD: /bin/systemctl restart olcrtc-server
olcpanel ALL=(ALL) NOPASSWD: /bin/systemctl status olcrtc-server
olcpanel ALL=(ALL) NOPASSWD: /bin/systemctl start olcrtc-server
olcpanel ALL=(ALL) NOPASSWD: /bin/systemctl stop olcrtc-server

# ⚠️ v2.0: iptables-правила для учёта трафика УДАЛЕНЫ (Решение 5 — трафик не считается).
# Эти строки больше НЕ нужны; не добавляйте их:
#   olcpanel ... /sbin/iptables -t mangle ...   ← удалено

# journalctl для чтения логов OlcRTC
olcpanel ALL=(ALL) NOPASSWD: /bin/journalctl -u olcrtc-server *
```

> Управление доступом в v2.0 идёт **не через iptables**, а через запись `authz.json` (права на файл/каталог `data/`, не sudo) и `systemctl restart olcrtc-server` (после валидации файла, см. 7.10.2).

```bash
# Установить с правильными правами (обязательно visudo для проверки синтаксиса)
visudo -c -f /etc/sudoers.d/olcpanel
chmod 440 /etc/sudoers.d/olcpanel
```

---

## 7.8 Скрипт установки с нуля (install.sh)

```bash
#!/bin/bash
# /opt/olcpanel/scripts/install.sh
# Полная установка OlcPanel с нуля на Ubuntu 22.04 LTS
# Запускать от root

set -euo pipefail

# ─── Цвета для вывода ────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; NC='\033[0m'; BOLD='\033[1m'

log_info()    { echo -e "${GREEN}[INFO]${NC}  $*"; }
log_warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
log_error()   { echo -e "${RED}[ERROR]${NC} $*"; }
log_section() { echo -e "\n${BOLD}${BLUE}══════ $* ══════${NC}\n"; }

# ─── Проверки ─────────────────────────────────────────────────────────────────
if [ "$EUID" -ne 0 ]; then
    log_error "Run as root: sudo bash install.sh"
    exit 1
fi

if [ ! -f /etc/os-release ] || ! grep -q "Ubuntu 22" /etc/os-release; then
    log_warn "This script is tested on Ubuntu 22.04 only. Proceeding anyway..."
fi

PANEL_DIR="/opt/olcpanel"
if [ ! -d "$PANEL_DIR" ]; then
    log_error "Panel directory not found: $PANEL_DIR"
    log_error "Clone/copy the project to $PANEL_DIR first"
    exit 1
fi

if [ ! -f "$PANEL_DIR/.env" ]; then
    log_error ".env file not found at $PANEL_DIR/.env"
    log_error "Copy .env.example to .env and fill in the values"
    exit 1
fi

# ─── Переменные ───────────────────────────────────────────────────────────────
DOMAIN=""
PANEL_USER="olcpanel"

# Читаем домен из .env
if grep -q "PANEL_URL" "$PANEL_DIR/.env"; then
    PANEL_URL=$(grep "^PANEL_URL=" "$PANEL_DIR/.env" | cut -d'=' -f2 | tr -d '"')
    DOMAIN=$(echo "$PANEL_URL" | sed 's|https://||' | sed 's|http://||' | sed 's|/.*||')
fi

if [ -z "$DOMAIN" ]; then
    read -rp "Enter your panel domain (e.g. panel.example.com): " DOMAIN
fi

log_info "Domain: $DOMAIN"
log_info "Panel dir: $PANEL_DIR"

# ─── ШАГ 1: Системные пакеты ──────────────────────────────────────────────────
log_section "Step 1: System packages"

apt-get update -qq
apt-get install -y -qq \
    python3.11 \
    python3.11-venv \
    python3.11-dev \
    python3-pip \
    nginx \
    certbot \
    python3-certbot-nginx \
    sqlite3 \
    iptables \
    curl \
    wget \
    git \
    logrotate \
    fail2ban

log_info "Packages installed"

# ─── ШАГ 2: Системный пользователь ───────────────────────────────────────────
log_section "Step 2: System user"

if id "$PANEL_USER" &>/dev/null; then
    log_info "User $PANEL_USER already exists"
else
    useradd \
        --system \
        --home "$PANEL_DIR" \
        --shell /sbin/nologin \
        --comment "OlcPanel service user" \
        "$PANEL_USER"
    log_info "User $PANEL_USER created"
fi

# Добавляем в группу systemd-journal для чтения логов
usermod -a -G systemd-journal "$PANEL_USER"

# ─── ШАГ 3: Python virtualenv ─────────────────────────────────────────────────
log_section "Step 3: Python virtualenv"

if [ ! -d "$PANEL_DIR/venv" ]; then
    python3.11 -m venv "$PANEL_DIR/venv"
    log_info "Virtualenv created"
else
    log_info "Virtualenv already exists"
fi

"$PANEL_DIR/venv/bin/pip" install --quiet --upgrade pip
"$PANEL_DIR/venv/bin/pip" install --quiet -r "$PANEL_DIR/requirements.txt"
log_info "Python dependencies installed"

# ─── ШАГ 4: Директории и права ────────────────────────────────────────────────
log_section "Step 4: Directories and permissions"

mkdir -p "$PANEL_DIR/data"
mkdir -p "$PANEL_DIR/logs"
mkdir -p "$PANEL_DIR/backups"
mkdir -p /var/www/certbot

# Права
chown -R "$PANEL_USER:$PANEL_USER" "$PANEL_DIR"
chmod 700 "$PANEL_DIR/data"
chmod 755 "$PANEL_DIR/logs"
chmod 700 "$PANEL_DIR/backups"
chmod 600 "$PANEL_DIR/.env"
chmod +x "$PANEL_DIR/scripts/"*.sh

log_info "Directories and permissions set"

# ─── ШАГ 5: Nginx ─────────────────────────────────────────────────────────────
log_section "Step 5: Nginx configuration"

# Создаём proxy_params.conf
cat > /etc/nginx/conf.d/proxy_params.conf << 'PROXY_PARAMS'
proxy_http_version 1.1;
proxy_set_header Host              $host;
proxy_set_header X-Real-IP         $remote_addr;
proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
proxy_set_header X-Forwarded-Proto $scheme;
proxy_set_header Connection        "";
proxy_connect_timeout   10s;
proxy_send_timeout      30s;
proxy_read_timeout      30s;
proxy_buffering         on;
proxy_buffer_size       4k;
proxy_buffers           8 4k;
proxy_set_header X-Client-IP $remote_addr;
PROXY_PARAMS

# Создаём временный HTTP конфиг (без HTTPS — для certbot)
cat > /etc/nginx/conf.d/olcpanel-temp.conf << NGINX_TEMP
limit_req_zone \$binary_remote_addr zone=login:10m rate=5r/m;
limit_req_zone \$binary_remote_addr zone=api:10m   rate=60r/m;

server {
    listen 80;
    server_name ${DOMAIN};
    root ${PANEL_DIR}/static;

    location /.well-known/acme-challenge/ {
        root /var/www/certbot;
        allow all;
    }

    location / { return 200 "OlcPanel installing..."; }
}
NGINX_TEMP

# Проверяем синтаксис и перезапускаем
nginx -t
systemctl reload nginx 2>/dev/null || systemctl start nginx
log_info "Nginx temporary config applied"

# ─── ШАГ 6: Let's Encrypt ─────────────────────────────────────────────────────
log_section "Step 6: SSL Certificate (Let's Encrypt)"

if [ -f "/etc/letsencrypt/live/${DOMAIN}/fullchain.pem" ]; then
    log_info "Certificate already exists for $DOMAIN"
else
    read -rp "Enter email for Let's Encrypt notifications: " LE_EMAIL
    certbot certonly \
        --webroot \
        --webroot-path /var/www/certbot \
        --email "$LE_EMAIL" \
        --agree-tos \
        --no-eff-email \
        -d "$DOMAIN" \
        --non-interactive

    log_info "Certificate obtained for $DOMAIN"
fi

# Создаём deploy hook для автообновления
mkdir -p /etc/letsencrypt/renewal-hooks/deploy
cat > /etc/letsencrypt/renewal-hooks/deploy/reload-nginx.sh << 'HOOK'
#!/bin/bash
systemctl reload nginx
echo "Nginx reloaded after cert renewal at $(date)" >> /var/log/certbot-nginx-reload.log
HOOK
chmod +x /etc/letsencrypt/renewal-hooks/deploy/reload-nginx.sh

# ─── ШАГ 7: Финальный Nginx конфиг ───────────────────────────────────────────
log_section "Step 7: Final Nginx config with HTTPS"

rm -f /etc/nginx/conf.d/olcpanel-temp.conf

sed "s/your-panel-domain.com/${DOMAIN}/g" \
    "$PANEL_DIR/nginx/olcpanel.conf.template" \
    > /etc/nginx/conf.d/olcpanel.conf

nginx -t
systemctl reload nginx
log_info "Final Nginx config applied with HTTPS"

# ─── ШАГ 8: sudoers ───────────────────────────────────────────────────────────
log_section "Step 8: sudoers"

cp "$PANEL_DIR/systemd/sudoers-olcpanel" /etc/sudoers.d/olcpanel
chmod 440 /etc/sudoers.d/olcpanel
visudo -c -f /etc/sudoers.d/olcpanel
log_info "sudoers configured"

# ─── ШАГ 9: systemd юниты ─────────────────────────────────────────────────────
log_section "Step 9: systemd units"

# OlcPanel service
cp "$PANEL_DIR/systemd/olcpanel.service" /etc/systemd/system/olcpanel.service

# Backup timer
cp "$PANEL_DIR/systemd/olcpanel-backup.service" /etc/systemd/system/olcpanel-backup.service
cp "$PANEL_DIR/systemd/olcpanel-backup.timer"   /etc/systemd/system/olcpanel-backup.timer

systemctl daemon-reload
systemctl enable olcpanel.service
systemctl enable olcpanel-backup.timer
log_info "systemd units registered"

# ─── ШАГ 10: База данных ──────────────────────────────────────────────────────
log_section "Step 10: Database initialization"

# Запускаем Alembic миграции от имени olcpanel
sudo -u "$PANEL_USER" \
    "$PANEL_DIR/venv/bin/alembic" \
    --config "$PANEL_DIR/alembic.ini" \
    upgrade head

log_info "Database migrations applied"

# Создаём первого администратора
log_info "Creating admin user..."
sudo -u "$PANEL_USER" \
    "$PANEL_DIR/venv/bin/python" \
    "$PANEL_DIR/scripts/create_admin.py"

# ─── ШАГ 11: Запуск сервисов ──────────────────────────────────────────────────
log_section "Step 11: Starting services"

systemctl start olcpanel.service
sleep 3

if systemctl is-active --quiet olcpanel.service; then
    log_info "OlcPanel service started successfully"
else
    log_error "OlcPanel service failed to start!"
    journalctl -u olcpanel.service -n 20 --no-pager
    exit 1
fi

systemctl start olcpanel-backup.timer

# Проверяем health
HEALTH=$(curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:8000/api/health)
if [ "$HEALTH" = "200" ]; then
    log_info "Health check: OK (HTTP 200)"
else
    log_warn "Health check returned HTTP $HEALTH"
fi

# ─── ШАГ 12: Логротация ───────────────────────────────────────────────────────
log_section "Step 12: Log rotation"

cat > /etc/logrotate.d/olcpanel << LOGROTATE
${PANEL_DIR}/logs/*.log {
    daily
    missingok
    rotate 14
    compress
    delaycompress
    notifempty
    create 640 ${PANEL_USER} ${PANEL_USER}
    sharedscripts
    postrotate
        systemctl kill -s HUP olcpanel.service 2>/dev/null || true
    endscript
}
LOGROTATE

log_info "Log rotation configured"

# ─── ШАГ 13: fail2ban ─────────────────────────────────────────────────────────
log_section "Step 13: fail2ban"

cat > /etc/fail2ban/jail.d/olcpanel.conf << FAIL2BAN
[olcpanel-login]
enabled  = true
port     = https
logpath  = /var/log/nginx/olcpanel-access.log
maxretry = 10
findtime = 600
bantime  = 3600
filter   = olcpanel-login
FAIL2BAN

cat > /etc/fail2ban/filter.d/olcpanel-login.conf << FILTER
[Definition]
failregex = ^<HOST>.*POST /api/auth/login HTTP/.*" 401
ignoreregex =
FILTER

systemctl enable fail2ban
systemctl restart fail2ban
log_info "fail2ban configured"

# ─── Итог ─────────────────────────────────────────────────────────────────────
log_section "Installation Complete!"

echo -e "
${GREEN}${BOLD}✅ OlcPanel installed successfully!${NC}

Access panel at: ${BOLD}https://${DOMAIN}${NC}

Services status:
  $(systemctl is-active olcpanel.service | grep -q active && echo "✅" || echo "❌") olcpanel.service
  $(systemctl is-active nginx | grep -q active && echo "✅" || echo "❌")          nginx
  $(systemctl is-active olcpanel-backup.timer | grep -q active && echo "✅" || echo "❌") olcpanel-backup.timer

⚠️  Security reminders:
  1. Remove ADMIN_PASSWORD from .env after first login
  2. Set up off-site backups (copy /opt/olcpanel/backups/ regularly)
  3. Check nginx logs: tail -f /var/log/nginx/olcpanel-access.log

📋 Useful commands:
  systemctl status olcpanel       — check panel status
  journalctl -u olcpanel -f       — live logs
  journalctl -u olcrtc-server -f  — OlcRTC logs
  bash /opt/olcpanel/scripts/backup.sh   — manual backup
  bash /opt/olcpanel/scripts/check_health.sh — health check
"
```

---

## 7.9 Скрипт проверки здоровья системы

```bash
#!/bin/bash
# /opt/olcpanel/scripts/check_health.sh
# Проверяет все компоненты системы

set -euo pipefail

GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[1;33m'; NC='\033[0m'
ok()   { echo -e "${GREEN}✅${NC} $*"; }
fail() { echo -e "${RED}❌${NC} $*"; FAILED=$((FAILED+1)); }
warn() { echo -e "${YELLOW}⚠️${NC}  $*"; }
FAILED=0

echo ""
echo "═══════ OlcPanel Health Check ═══════"
echo "Time: $(date -u)"
echo ""

# --- Сервисы ---
echo "── Services ──"

systemctl is-active --quiet olcpanel      && ok "olcpanel.service: running"     || fail "olcpanel.service: NOT running"
systemctl is-active --quiet nginx         && ok "nginx: running"                 || fail "nginx: NOT running"
systemctl is-active --quiet olcrtc-server && ok "olcrtc-server.service: running" || fail "olcrtc-server.service: NOT running"

# --- HTTP Health endpoint ---
echo ""
echo "── HTTP Health ──"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" --max-time 5 http://127.0.0.1:8000/api/health 2>/dev/null || echo "000")
[ "$HTTP_CODE" = "200" ] && ok "API health: HTTP 200" || fail "API health: HTTP $HTTP_CODE"

# --- Database ---
echo ""
echo "── Database ──"
DB="/opt/olcpanel/data/olcpanel.db"
if [ -f "$DB" ]; then
    DB_SIZE=$(du -h "$DB" | cut -f1)
    DB_CHECK=$(sqlite3 "$DB" "PRAGMA integrity_check;" 2>/dev/null || echo "error")
    [ "$DB_CHECK" = "ok" ] && ok "SQLite integrity: ok ($DB_SIZE)" || fail "SQLite integrity: $DB_CHECK"

    CLIENT_COUNT=$(sqlite3 "$DB" "SELECT COUNT(*) FROM clients WHERE deleted_at IS NULL;" 2>/dev/null || echo "?")
    ok "Clients in DB: $CLIENT_COUNT"
else
    fail "Database file not found: $DB"
fi

# --- TLS Certificate ---
echo ""
echo "── TLS Certificate ──"
DOMAIN=$(grep "^PANEL_URL=" /opt/olcpanel/.env 2>/dev/null | sed 's|.*https://||' | sed 's|/.*||' || echo "")
if [ -n "$DOMAIN" ]; then
    CERT_FILE="/etc/letsencrypt/live/${DOMAIN}/fullchain.pem"
    if [ -f "$CERT_FILE" ]; then
        CERT_EXPIRY=$(openssl x509 -enddate -noout -in "$CERT_FILE" | sed 's/notAfter=//')
        CERT_DAYS=$(( ( $(date -d "$CERT_EXPIRY" +%s) - $(date +%s) ) / 86400 ))
        if [ "$CERT_DAYS" -gt 14 ]; then
            ok "TLS cert expires in ${CERT_DAYS} days ($CERT_EXPIRY)"
        else
            warn "TLS cert expires in ${CERT_DAYS} days — renew soon!"
        fi
    else
        warn "Certificate file not found (may not be configured yet)"
    fi
fi

# --- Disk & Memory ---
echo ""
echo "── Resources ──"
DISK_USAGE=$(df -h /opt/olcpanel | tail -1 | awk '{print $5}')
DISK_PERCENT=$(echo "$DISK_USAGE" | tr -d '%')
[ "$DISK_PERCENT" -lt 80 ] && ok "Disk usage: $DISK_USAGE" || warn "Disk usage HIGH: $DISK_USAGE"

MEM_FREE=$(free -m | awk 'NR==2{printf "%d MB free (%.0f%%)", $7, $7*100/$2}')
ok "Memory: $MEM_FREE"

# --- Backups ---
echo ""
echo "── Backups ──"
BACKUP_DIR="/opt/olcpanel/backups"
if [ -d "$BACKUP_DIR" ]; then
    BACKUP_COUNT=$(find "$BACKUP_DIR" -name "*.db.gz" | wc -l)
    LAST_BACKUP=$(find "$BACKUP_DIR" -name "*.db.gz" -printf "%T@ %f\n" 2>/dev/null | sort -n | tail -1 | cut -d' ' -f2)
    if [ "$BACKUP_COUNT" -gt 0 ]; then
        ok "Backups: $BACKUP_COUNT files, latest: $LAST_BACKUP"
    else
        warn "No backups found in $BACKUP_DIR"
    fi
else
    warn "Backup directory not found"
fi

# --- OlcRTC ---
echo ""
echo "── OlcRTC ──"
if systemctl is-active --quiet olcrtc-server; then
    UPTIME=$(systemctl show olcrtc-server --property=ActiveEnterTimestamp | cut -d= -f2)
    ok "OlcRTC running since: $UPTIME"
    # Последние строки лога
    LAST_LOG=$(journalctl -u olcrtc-server -n 1 --no-pager --output=cat 2>/dev/null || echo "no logs")
    ok "Last log: $LAST_LOG"
fi

# --- Итог ---
echo ""
echo "════════════════════════════════════"
if [ "$FAILED" -eq 0 ]; then
    echo -e "${GREEN}${BOLD}All checks passed!${NC}"
else
    echo -e "${RED}${BOLD}$FAILED check(s) FAILED${NC}"
    exit 1
fi
```

---

## 7.10 Обновление OlcPanel (деплой новой версии)

```bash
#!/bin/bash
# Процедура обновления OlcPanel без downtime

PANEL_DIR="/opt/olcpanel"

# 1. Бэкап перед обновлением
echo "Creating pre-update backup..."
bash "$PANEL_DIR/scripts/backup.sh"

# 2. Забрать новый код (если git)
# git -C "$PANEL_DIR" pull origin main

# 3. Обновить зависимости
sudo -u olcpanel "$PANEL_DIR/venv/bin/pip" install \
    --quiet \
    -r "$PANEL_DIR/requirements.txt"

# 4. Применить миграции БД
sudo -u olcpanel "$PANEL_DIR/venv/bin/alembic" \
    --config "$PANEL_DIR/alembic.ini" \
    upgrade head

# 5. Перезапустить сервис (graceful: Uvicorn дождётся завершения текущих запросов)
systemctl restart olcpanel.service

# 6. Проверить что поднялся
sleep 3
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:8000/api/health)
if [ "$HTTP_CODE" = "200" ]; then
    echo "✅ Update successful! Panel is running."
else
    echo "❌ Panel failed to start after update! Rolling back..."
    # При необходимости: git -C "$PANEL_DIR" revert HEAD
    systemctl restart olcpanel.service
    exit 1
fi

# 7. Обновить статические файлы (Nginx отдаёт напрямую, перезагрузка не нужна)
echo "✅ Static files updated"

echo "Update complete."
```

---

## 7.11 Логи: что смотреть при проблемах

```bash
# ── Логи OlcPanel ────────────────────────────────────────────────────────────
journalctl -u olcpanel -f                          # live логи
journalctl -u olcpanel -n 100 --no-pager           # последние 100 строк
journalctl -u olcpanel --since "1 hour ago"        # за последний час
tail -f /opt/olcpanel/logs/app.log                 # файловый лог

# ── Логи OlcRTC ──────────────────────────────────────────────────────────────
journalctl -u olcrtc-server -f
journalctl -u olcrtc-server -n 50 --no-pager
# Поиск по ключевым словам:
journalctl -u olcrtc-server | grep -E "stall|bytes|liveness|ERROR|jingle"

# ── Логи Nginx ───────────────────────────────────────────────────────────────
tail -f /var/log/nginx/olcpanel-access.log
tail -f /var/log/nginx/olcpanel-error.log
# Ошибки 4xx/5xx:
grep " [45][0-9][0-9] " /var/log/nginx/olcpanel-access.log | tail -20

# ── Логи fail2ban ────────────────────────────────────────────────────────────
tail -f /var/log/fail2ban.log
fail2ban-client status olcpanel-login              # список забаненных IP

# ── Системные ресурсы ────────────────────────────────────────────────────────
htop                                               # CPU/RAM
df -h                                              # диск
du -sh /opt/olcpanel/data/                        # размер БД
du -sh /opt/olcpanel/logs/                        # размер логов

# ── SQLite: ручной запрос ────────────────────────────────────────────────────
sqlite3 /opt/olcpanel/data/olcpanel.db
  .tables
  SELECT id, name, status FROM clients;
  SELECT id, status, expires_at FROM subscriptions ORDER BY expires_at DESC LIMIT 5;
  .quit
```

---

## 7.12 Disaster Recovery

### 7.12.1 Восстановление из бэкапа

```bash
# 1. Найти последний бэкап
ls -lt /opt/olcpanel/backups/
# Пример: olcpanel-20260612.db.gz

# 2. Остановить панель
systemctl stop olcpanel

# 3. Создать резервную копию текущей (повреждённой) БД
cp /opt/olcpanel/data/olcpanel.db /opt/olcpanel/data/olcpanel.db.broken.$(date +%s)

# 4. Восстановить из бэкапа
gunzip -c /opt/olcpanel/backups/olcpanel-20260612.db.gz \
    > /opt/olcpanel/data/olcpanel.db

# 5. Проверить целостность
sqlite3 /opt/olcpanel/data/olcpanel.db "PRAGMA integrity_check;"
# Должно вернуть: ok

# 6. Установить права
chown olcpanel:olcpanel /opt/olcpanel/data/olcpanel.db
chmod 600 /opt/olcpanel/data/olcpanel.db

# 7. Запустить панель
systemctl start olcpanel

# 8. Проверить
bash /opt/olcpanel/scripts/check_health.sh
```

### 7.12.2 Полное переустановление с нуля

```bash
# Если нужно переехать на новый VPS:

# 1. На старом сервере: создать финальный бэкап
bash /opt/olcpanel/scripts/backup.sh
scp /opt/olcpanel/backups/olcpanel-$(date +%Y%m%d).db.gz user@new-server:/tmp/

# 2. Скопировать .env (без него не установить)
scp /opt/olcpanel/.env user@new-server:/tmp/olcpanel.env

# 3. На новом сервере:
#    - Склонировать репозиторий в /opt/olcpanel
#    - Скопировать .env
#    - Запустить install.sh
#    - Восстановить БД из бэкапа (шаги выше)
#    - Перегенерировать конфиги если сменился IP (через Settings → Jitsi server)
```

---

## 7.10 Эксплуатация `authz` / `Gate` (last-known-good) — операционные процедуры

> Прямое следствие Решения 4 / 5.2: `Gate` в проде работает по **last-known-good**. Это меняет процедуры рестарта, доставки и бэкапа. Все процедуры — аддитивны, не ломают общую комнату/ключ.

### 7.10.1 Конфигурация `server.yaml` (секция authz)

```yaml
authz:
  mode: allowlist        # off | allowlist | denylist (включать после привязки устройств владельца)
  file: /opt/olcrtc-panel/data/authz.json
  fail_mode: lkg         # ПРОД: lkg (last-known-good). open — только диагностика; closed — жёстко.
  lkg_max_age: 24h       # старше → critical-alert (+ опц. переход в closed как «потолок»)
  cold_start: closed     # нет валидного файла при старте → закрыть всех + громкий alert
  enforce_interval: 30s  # как часто рвать живые сессии заблокированных deviceId
```

> Включение гейта (сейчас `mode: off`): (1) клиент-владелец обновляет подписку в olcbox → `deviceId` привязался; (2) `cat .../authz.json` — убедиться, что hwid в `allow`; (3) выставить `mode: allowlist` + `fail_mode: lkg`; (4) `systemctl restart olcrtc-server`; (5) проверить: своё устройство работает, «Блок» в панели рвёт чужое ≤ `enforce_interval`.

### 7.10.2 Процедура рестарта `olcrtc` (ОБЯЗАТЕЛЬНО валидировать файл)

LKG живёт в памяти процесса. Рестарт при битом/пустом файле = потеря LKG → `cold_start: closed` отрубит всех. Поэтому:

```bash
# /opt/olcpanel/scripts/safe_restart_olcrtc.sh
AUTHZ=/opt/olcrtc-panel/data/authz.json
# 1. файл существует и парсится как JSON со схемой?
if ! jq -e '.version and .allow' "$AUTHZ" >/dev/null 2>&1; then
    echo "ABORT: authz.json невалиден — рестарт отменён (иначе cold-start closed = блок всех)"
    exit 1
fi
# 2. (опц.) допустимое число allow (защита от пустого среза по ошибке)
CNT=$(jq '.allow | length' "$AUTHZ")
[ "$CNT" -eq 0 ] && echo "WARN: allow пуст — подтвердите, что это намеренно"
# 3. рестарт
sudo systemctl restart olcrtc-server
```

> При замене бинаря: `mv` (не `cp` — даёт «Text file busy»); старый бинарь — в `/root/olcrtc-backups/`. После рестарта проверить `/health`: `applied_version` совпал, `load_errors=0`.

### 7.10.3 Доставка `authz.json` на сервер (single → fleet)

```
Single-server (MVP): authz_writer пишет файл локально, атомарно:
  write tmp → fsync → os.rename(tmp, authz.json)   # rename атомарен в пределах ФС
  Gate перечитает по mtime. Никакой сети.

Fleet (Этап 2, вариант A — rsync+trigger):
  1. панель генерирует payload (version из authz_state per server_id)
  2. rsync payload → /opt/olcrtc-panel/data/authz.json.tmp на целевом сервере
  3. trigger (HTTP/systemd-path) на сервере: fsync + os.rename(.tmp, authz.json)
     — атомарность ОБЯЗАТЕЛЬНА; «приехавший» tmp не виден Gate до rename
  4. ServerHealthService.poll(/health) подтверждает applied_version == отправленной

Fleet (Этап 3, вариант B — signed HTTP):
  POST /internal/authz/update (mTLS/подпись); сервер: version-reject (≤applied →409),
  валидация схемы (битый →400, LKG не трогается), атомарная запись, 200 ТОЛЬКО после записи.
```

### 7.10.4 Health-мониторинг и эскалация устаревания

```
ServerHealthService → GET /health → пишет в servers:
  applied_authz_version, lkg_valid_at, load_errors, last_health_at.
Эскалация (→ «Требуют внимания» + Telegram-alert, Этап 2):
  • applied < expected            → доставка не подтверждена (split-brain, 5.3)
  • load_errors > 0               → Gate на LKG из-за битого файла → re-push (4.11)
  • now - lkg_valid_at > lkg_max_age (stale:true) → устаревший allowlist
```

### 7.10.5 Бэкап включает `authz.json`

`backup.sh` дополнительно копирует `authz.json` и `server.yaml` рядом с дампом SQLite:

```bash
cp /opt/olcrtc-panel/data/authz.json   "${BACKUP_DIR}/authz-${TIMESTAMP}.json"
cp /root/olcrtc/server.yaml            "${BACKUP_DIR}/server-${TIMESTAMP}.yaml"
```

> Зачем: SQLite — источник, из которого `authz.json` **пересобирается** (5.2/5.4). Но снимок `authz.json`/`server.yaml` ускоряет восстановление и даёт точку отката allowlist. При полном восстановлении: поднять БД → `authz_writer.write_authz_file` (перегенерит файл с новым version) → валидация (7.10.2) → рестарт.

---

*Следующий раздел: [[OlcPanel_08_Безопасность]] — модель угроз (вкл. «потеря authz.json ≠ открытый доступ»), JWT, 2FA/секретный путь, подпись push, SPOF.*

════════════════════════════════════════════════════════════════════════════════
<!-- Конец файла 07_Инфраструктура.md -->
════════════════════════════════════════════════════════════════════════════════
