# INDEX — карта репозитория olcRTC (VPN-ядро, Go)

> Карта для навигации Claude по ядру: с неё начинай сессию, чтобы понять «куда идти за чем».
> Обновляй при изменении структуры (см. правило внизу). **⭐ = источник правды (канон/контракт).**
> Это карта, а не документация — только опорные файлы. Точное состояние кода — в `graphify-out/graph.json`
> (`graphify query "..."`). Правила репо и стиль — в `CLAUDE.md` (mage, ветка `master`, WTFPL).
>
> ⚠️ **Источники правды тут другие, чем в панели.** Панель — контроль (пишет `authz.json`/`server.yaml`),
> ядро — исполнение (читает и применяет). Контракты с панелью помечены ⭐.

## Корень
- `CLAUDE.md` — инструкции репо (сборка `mage`, стиль Go, провенанс). `readme.md` — обзор проекта.
- `go.mod` — модуль `github.com/openlibrecommunity/olcrtc`, Go 1.26.3. `LICENSE` — WTFPL.
- `magefile.go` ⭐ — единая точка сборки/проверок (см. ниже). `.golangci.yml` — конфиг линтера (v2).
- `.github/workflows/ci.yml` — CI (lint + тесты). `SECURITY.md`, `CONTRIBUTING.md` — политики.

## Сборка и проверки (mage)
- `mage check` ⭐ — полный гейт: Build + Vet + Lint + TestFull. Гоняй перед коммитом.
- `mage build` — сборка; `mage lint` — golangci-lint v2 (0 issues); `mage test` — быстрые тесты `-race`.
- `mage testFull` / `mage e2e` — полный прогон / end-to-end (`internal/e2e/`).

## cmd/ — точки входа (CLI)
- `cmd/olcrtc/main.go` ⭐ — основной бинарь: подкоманды `srv` (узел-сервер) и клиент.
  Рядом `stderr_filter_*.go` — платформенная фильтрация вывода.
- `cmd/olcrtc-cgo/main.go` — cgo-вариант сборки (для мобильной/встраиваемой линковки).

## internal/ — основная логика (приватная)
- `server/server.go` ⭐ — **серверная сторона узла**: `Run()`, control-loops, `peerSession`. Сердце `srv`.
- `config/config.go` ⭐ — **читает `server.yaml`/профиль** (room, provider, crypto, transport, secrets).
  Это приёмник того, что рендерит панель. `Load()`/`Apply()`/`ApplyProfile()`.
- `authz/authz.go` ⭐ — **allowlist deviceId** (контракт `authz.json` с панелью): кого пускать/блокировать.
- `engine/` ⭐ — **движки провайдеров = «несущая.provider» панели**: `engine.go` + `jitsi/`, `livekit/`,
  `goolom/` (telemost-класс: signaling/media/session), `builtin/`. Выбор провайдера транспорта.
- `transport/` ⭐ — **каналы = «несущая.transport» панели**: `transport.go`, `traffic.go` +
  `datachannel/`, `videochannel/`, `vp8channel/`, `seichannel/`.
- `crypto/chacha.go` ⭐ — крипто-транспорт (ChaCha); `handshake/handshake.go` — рукопожатие сессии.
- `client/client.go` — клиентская сторона туннеля. `runtime/runtime.go` — рантайм + health-tracker.
- Прочие слои (служебные): `control/`, `supervisor/`, `framing/`, `muxconn/`, `protect/` (VPN-protect
  сокета), `names/` (генерация имён из `data/`), `logger/`, `app/`. Роль ясна из имени; детали — в графе.

## pkg/olcrtc/ — публичный API (экспортируемый)
- `olcrtc.go` ⭐ — публичная обёртка: `Session`, `.Dial()` (то, чем пользуются внешние потребители).
- `conn.go` — реализация `net.Conn` поверх WebRTC. `tunnel/` — обёртка туннеля.

## mobile/ — биндинг для клиента
- `mobile.go` ⭐ — gomobile-биндинг ядра. **Именно его встраивает `olcbox-src`** (`:sharedUI:olcrtc-bin`).

## docs/ — документация ядра (перед кодом читать `docs/*.md`)
- `uri.md` ⭐, `settings.md` ⭐ — **форматы URI/YAML** (byte-parity: панель обязана совпадать байт-в-байт).
- `configuration.md`, `manual.md`, `sub.md`, `fast.md` — конфигурация/подписка/режимы.
- `bridge_PROTOCOL.md` ⭐ — протокол моста. `about.md` — общее описание.
- `arch/`, `Architecture olcRTC panel/` — зеркала архитектуры панели (справочно, канон панели — в её репо).

## Прочее (служебное, не логика ядра)
- `code/` — Python-POC реверса telemost (research, не билдится в бинарь). `script/` — `srv.sh`, `cnc.sh` (хелперы).
- `data/` — словари `names`/`surnames` для `internal/names`. `graphify-out/` — граф кода.

## Быстрая навигация
- Поведение узла-сервера → `internal/server/server.go`; запуск CLI → `cmd/olcrtc/main.go`.
- Разобрать/поменять чтение конфига узла → `internal/config/config.go` (контракт `server.yaml`).
- Блокировка/допуск клиента → `internal/authz/authz.go` (контракт `authz.json` с панелью).
- Добавить/править провайдера (несущую) → `internal/engine/<provider>/`; транспорт → `internal/transport/`.
- Крипто/рукопожатие → `internal/crypto/`, `internal/handshake/`.
- Публичный API для встраивания → `pkg/olcrtc/`; мобильный биндинг → `mobile/mobile.go`.
- Формат URI/YAML (сверка с панелью) → `docs/uri.md ⭐`, `docs/settings.md ⭐`.

---
> **Правило:** обновлять этот INDEX при добавлении/удалении/переезде значимых пакетов или папок.
