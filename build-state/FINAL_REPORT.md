# OlcPanel Final Build Report — 14.06.2026

## Статус: ВСЕ 18 ЗАДАЧ ВЫПОЛНЕНЫ

Построен на основе архитектурных документов v2.0 (`docs/Architecture olcRTC panel/OlcPanel_00..08.md`).
Сборка: Grok-loop (systemd-timer, Tasks 1-9) + Claude Code (Tasks 10-18).

---

## Этап 1 — Монетизация + Фундамент

### Task 1: LKG Gate (`internal/authz/authz.go`)
Gate с Last-Known-Good кэшем. При битом/отсутствующем authz.json держит последний валидный allow/deny.
Ключевые поля: `lkgAllow`, `lkgDeny`, `lkgValidAt`, `loadErrors`, `lastVersion`.
Config: `fail_mode: lkg|open|closed` (прод=lkg), `lkg_max_age`.
Версионный контракт: строгий monotonic — `version <= lastVersion` → reject.
Тесты: `go test ./internal/authz/... -v` → PASS 8/8. `go vet ./...` → clean.

### Task 2: authz_writer — монотонный version + authz_state
`services/authz_writer.py` — атомарная запись (tmp+fsync+rename). Версия из таблицы `authz_state`:
читает, бампает, коммитит перед записью файла.

### Task 3: Схема БД v2.0 + миграции
Эволюционные миграции в `main.py`. Добавлены: `servers` (health-поля), `device_binding` расширен
(`server_id`, `is_active`), `authz_state`. Backfill: device_binding → server_id=1, is_active=1.

### Task 4: Авто-блок по сроку
APScheduler `check_expiry_dates()` в `scheduler.py`: ставит `client.status=expired` +
`device_binding.is_active=0` + вызывает `write_authz_file()` + логирует в `audit_log`.

### Task 5: Скелет флота (ServerRegistry, Coordinator, LocalFilePusher)
`services/coordinator.py` — `MultiServerAuthzCoordinator.on_change(server_id)` → slice по
`user_devices.server_id` → `LocalFilePusher`. Feature flag: `MULTI_SERVER_AUTHZ=off`.

---

## Этап 2 — Операционка

### Task 6: Pusher A (RsyncPusher + health-poll)
`services/pusher.py` — `RsyncPusher` доставляет authz.json атомарно (tmp → fsync → os.replace).
`services/health_service.py` — `ServerHealthService.poll()` обновляет applied_version/load_errors.

### Task 7: Telegram-уведомления
`services/notification.py` — `NotificationService` через stdlib urllib.
События: client.created/extended/blocked, device.soft_block, authz.push_failed, server.down, expiry.warn.
Конфиг: TELEGRAM_BOT_TOKEN, TELEGRAM_CHAT_ID, WARN_DAYS_BEFORE.

### Task 8: Авто-бэкап БД
`services/backup.py` — sqlite3 hot backup + cp authz.json + tar.gz → `backups/YYYYMMDD_HHMMSS/`.
Retention cleanup. APScheduler@02:00.

### Task 9: Безопасность входа
TOTP 2FA (pyotp), SECRET_PATH middleware (без него → 404), rate limit (5r/min, ban 5min),
fail2ban (maxretry=3, bantime=3600), security headers (X-Content-Type-Options, X-Frame-Options,
X-XSS-Protection, Referrer-Policy).

---

## Этап 3 — Расширение

### Task 10: UA-routing `/api/sub/{token}`
`routers/subscription.py` — три выхода по UA/Accept:
- Браузер → HTMLResponse (dark page + SVG QR, qrcode lib)
- curl/API → PlainTextResponse (sub.md: #name, #update, #refresh, #expires, olcrtc:// lines)
- Accept:application/json → JSONResponse (LocationBundleV4)
Все пути привязывают x-hwid device к клиенту.

### Task 11: Pusher B — Go admin HTTP server
Go-сторона (`internal/authz/admin_server.go`):
- GET /health → JSON {mode, fail_mode, applied_version, lkg_valid_at, load_errors, allow_count, stale}
- POST /internal/authz/update (Bearer) → Gate.WriteAuthz() атомарная запись
Запускается если authz.admin_addr != "" в конфиге (OLCRTC_ADMIN_ADDR).
Панель-сторона: `services/coordinator.py` — HttpPusher постит на {api_base}/internal/authz/update.

### Task 12: Fleet dashboard
Backend: `routers/servers.py` — GET/POST/PATCH/DELETE /api/servers,
POST /api/servers/{id}/repush-authz (202), GET /api/servers/{id}/health.
Frontend: Tab "Fleet" в index.html + js/app.js (loadFleet, renderFleet, repushAuthz).
Traffic-light: green (applied==expected && errors==0), yellow (stale), red (errors>0).

### Task 13: Миграция устройств между серверами
POST /api/clients/{id}/devices/{device_id}/move.
Fail-safe порядок: on_change(to_server_id) → update binding → on_change(from_server_id).
crud.move_device_server().

### Task 14: RBAC
role поле на User, require_owner dependency (403 если не owner), GET /api/auth/me.

### Task 15: Webhooks
GET/POST/DELETE /api/settings/webhooks.
Webhook(id, url, secret, events JSON, is_active).
WebhookService.fire() с HMAC-SHA256 (X-OlcPanel-Signature).

---

## Task 16: Крон-тик + lock + resume
build-state/tick.sh с flock (run.lock). build-state/RESUME_PROMPT.md.
olcrtc-grok-tick.timer — остановлен после завершения сборки.

---

## Task 17: Git hygiene + CI smoke

Go-репо (feat/lkg-gate-authz):
- go vet ./... → CLEAN
- go test ./internal/authz/... -v → 8/8 PASS
- .gitignore обновлён (build artifacts, runtime data, graphify-out, *.bak)

Panel-репо (main, GitHub: asadulakurbanov4-web/olcRTC-panel):
- Коммиты: 1dfdded (Tasks 1-9), 6f71724 (Task 10), 7c7ece9 (Tasks 11-15, Stage 3)
- Запушено в main.

---

## Task 18: Процедура активации Gate

ВНИМАНИЕ: Gate сейчас `mode: off` — все клиенты проходят. Активация вручную.

Предварительные условия:
1. device_id владельца привязан через OlcBox (x-hwid) и есть в authz.json allow.
2. Проверить: sqlite3 /opt/olcrtc-panel/data/panel.db "SELECT device_id, client_id FROM device_binding WHERE is_active=1;"
3. cat /opt/olcrtc-panel/data/authz.json

Шаги:
1. Бэкап конфига: cp /root/olcrtc/config.yaml /root/olcrtc/config.yaml.pre-allowlist
2. Проверить health: curl -s http://127.0.0.1:8081/health
   (ожидаем: mode=off, fail_mode=lkg, allow_count > 0)
3. Переключить в config.yaml: authz.mode: allowlist
4. systemctl restart olcrtc
5. Проверить что device владельца проходит (подключиться с OlcBox)
6. Откат если что-то не так:
   cp /root/olcrtc/config.yaml.pre-allowlist /root/olcrtc/config.yaml
   systemctl restart olcrtc

После активации:
- Новые клиенты через панель → автоматически в authz.json (Task 2 + coordinator)
- Блокировка по сроку работает автоматически (Task 4 + scheduler)
- Fleet tab показывает applied_version == expected_version

---

## Живые сервисы
- 6 клиентов активны, туннели работают
- olcrtc + olcrtc-panel active (systemd)
- Disk: ~87% (после чисток, было 94%)

## Архитектурные документы
9 файлов OlcPanel_00..08.md переписаны под v2.0 (deviceId-модель, LKG Gate, fleet, Pusher A/B,
RBAC, webhooks). Закоммичены в Go-репо.
