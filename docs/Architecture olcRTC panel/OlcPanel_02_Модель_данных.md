════════════════════════════════════════════════════════════════════════════════
<!-- ФАЙЛ 3/10: 02_Модель_данных.md -->
════════════════════════════════════════════════════════════════════════════════

# OlcPanel — Раздел 2: Модель данных
## ERD · Таблицы SQLite · Поля · Типы · Индексы · Связи · Примеры данных

> **Версия:** 2.0 (каноническая модель `deviceId` + `server_id` + authz/health-поля; `configs`/`traffic_*` помечены LEGACY/DORMANT) | **Статус:** 🟡 В разработке
> **СУБД:** SQLite 3 (WAL mode, foreign keys enabled)
> **ORM:** SQLAlchemy 2.0 (declarative base) + Alembic (миграции)
> **Зависимости:** [[OlcPanel_00_Введение]] (Решения 1–5), [[OlcPanel_01_Обзор_и_требования]], раздел 4 и 5.2 в [[OlcPanel_Анализ_Marzban_3xui]]

> ⚠️ **Что изменилось в v2.0 модели данных.**
> 1. **Идентичность клиента — `user_devices.device_id` (`x-hwid`), а не пер-клиентский `configs.crypto_key`.** Ключ и комната общие (из `server.yaml`). Таблица `configs` сохранена как **LEGACY** (история/совместимость), но не является источником доступа.
> 2. **`server_id`** добавлен в `subscriptions` и `user_devices` (семантика — 2.1.4) как фундамент флота с Этапа 1.
> 3. **`servers`** получает **authz/health-поля** (`applied_authz_version`, `lkg_valid_at`, `load_errors`, `fail_mode`, `last_health_at`) — прямое следствие решения **last-known-good** (5.2): свежесть применённого allowlist становится наблюдаемой величиной.
> 4. Добавлена панельная таблица **`authz_state`** — монотонный счётчик `version`, который пишется в `authz.json` и проверяется сервером на version-reject.
> 5. **`traffic_daily`/`traffic_snapshots`** помечены **DORMANT** (Решение 5: пер-клиентский трафик не считается на WebRTC-туннеле).

---

## 2.1 Обзор модели данных

### 2.1.1 Список таблиц (v2.0)

**Канонические (активно используются):**

| # | Таблица | Назначение | Ключевое для v2.0 |
|---|---------|------------|-------------------|
| 1 | `admins` | Учётные данные администратора | bcrypt + `fp`-инвалидатор |
| 2 | `plans` | Тарифные планы (продажа по сроку) | `traffic_gb` — спящее поле |
| 3 | `clients` | Клиенты VPN-сервиса | soft-block, не delete |
| 4 | `subscriptions` | Подписки клиентов | **+`server_id`** |
| 5 | `user_devices` | **Устройства (deviceId/x-hwid) — ядро доступа** | **+`server_id`**, мост к `authz.json` |
| 6 | `servers` | Реестр серверов флота | **+authz/health-поля (LKG)** |
| 7 | `authz_state` | **Версия allowlist (монотонный счётчик)** | пишется в `authz.json` |
| 8 | `payments` | История платежей | — |
| 9 | `server_settings` | Глобальные настройки/пути | `warn_days_before`, `authz_*` |
| 10 | `audit_log` | Журнал действий + authz-push | предусловие multi-server (5.6) |

**Legacy / Dormant (сохранены для совместимости, НЕ источник истины):**

| Таблица | Статус | Почему |
|---|---|---|
| `configs` | **LEGACY** | Пер-клиентский `crypto_key`/комната отвергнуты (Решение 1). Хранит историю старых конфигов; новый доступ через `user_devices`+`authz.json`. |
| `traffic_daily` | **DORMANT** | Пер-клиентский трафик не считается (Решение 5). Схема оставлена под будущую серверную телеметрию (Этап 3). |
| `traffic_snapshots` | **DORMANT** | Зависела от iptables-аккаунтинга, которого в модели нет. |

### 2.1.2 ERD-диаграмма (Entity Relationship Diagram) — текущая (до эволюции)

```
┌─────────────┐         ┌─────────────────┐         ┌─────────────┐
│   admins    │         │     clients     │         │    plans    │
├─────────────┤         ├─────────────────┤         ├─────────────┤
│ id (PK)     │         │ id (PK)         │    ┌───▶│ id (PK)     │
│ username    │         │ name            │    │    │ name        │
│ password_hash│        │ telegram        │    │    │ description │
│ created_at  │         │ phone           │    │    │ price       │
│ last_login  │         │ notes           │    │    │ duration_days│
└─────────────┘         │ status          │    │    │ traffic_gb  │
                        │ created_at      │    │    │ is_active   │
                        │ updated_at      │    │    │ created_at  │
                        │ deleted_at      │    │    └─────────────┘
                        └────────┬────────┘    │
                                 │ 1           │
                                 │             │
              ┌──────────────────┼─────────────┘
              │                  │
              │ N                │ N
    ┌─────────▼──────┐  ┌────────▼────────┐
    │    configs     │  │  subscriptions  │
    ├────────────────┤  ├─────────────────┤
    │ id (PK)        │  │ id (PK)         │
    │ client_id (FK) │  │ client_id (FK)  │──────▶ clients.id
    │ crypto_key     │  │ plan_id (FK)    │──────▶ plans.id
    │ room_url       │  │ status          │
    │ room_name      │  │ started_at      │
    │ jitsi_server   │  │ expires_at      │
    │ uri            │  │ traffic_used_mb │
    │ status         │  │ traffic_limit_mb│
    │ created_at     │  │ created_at      │
    │ revoked_at     │  │ updated_at      │
    │ revoke_reason  │  └────────┬────────┘
    └────────────────┘           │ 1
                                 │
                                 │ N
                        ┌────────▼────────┐
                        │    payments     │
                        ├─────────────────┤
                        │ id (PK)         │
                        │ subscription_id │──▶ subscriptions.id
                        │ client_id (FK)  │──▶ clients.id
                        │ amount          │
                        │ currency        │
                        │ method          │
                        │ period_days     │
                        │ note            │
```

### 2.1.3 Каноническая v2.0-схема ядра доступа (`user_devices`, `servers`, `authz_state`)

> Ниже — три таблицы, на которых держится монетизация и масштабирование v2.0. Они **дополняют** существующие `clients/subscriptions/plans/payments/audit_log`, не заменяя их (`clients` остаётся «человеком», `user_devices` — его устройствами). SQLite-диалект приведён в 2.5; ниже — логическая схема с акцентом на новые поля.

#### A. `user_devices` — устройства клиента (ядро доступа, мост к `authz.json`)

```sql
CREATE TABLE user_devices (
    id            INTEGER  PRIMARY KEY AUTOINCREMENT,
    client_id     INTEGER  NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    device_id     TEXT     NOT NULL UNIQUE,        -- x-hwid из olcbox; ЕДИНИЦА ИДЕНТИЧНОСТИ
    label         TEXT     NULL,                   -- «телефон Антона», для UI
    server_id     INTEGER  NULL REFERENCES servers(id) ON DELETE SET NULL, -- куда ДОСТАВЛЯТЬ allow
    is_active     BOOLEAN  NOT NULL DEFAULT 1,     -- участвует ли в allow[] (soft-block снимает)
    last_used_at  DATETIME NULL,                   -- момент последнего допуска/handshake (если есть телеметрия)
    created_at    DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_user_devices_client     ON user_devices(client_id);
CREATE INDEX idx_user_devices_server     ON user_devices(server_id);
CREATE INDEX idx_user_devices_active     ON user_devices(is_active);
```

- `device_id` UNIQUE — то, что попадает в `authz.json.allow[]`. `Gate.Allowed(device_id)` решает доступ.
- Множество `allow` для сервера X = `SELECT device_id FROM user_devices d JOIN clients c ON ... JOIN subscriptions s ... WHERE d.is_active AND d.server_id = X AND s.status='active' AND s.expires_at > now()`. Это «рабочий ключ» координатора (см. 2.1.4).
- **Связь с Phase 1:** любое изменение (`create/extend/block/unblock/delete`, sub-fetch, scheduler) → `authz_writer.write_authz_file(db)` пересобирает `allow` из активных `user_devices` и пишет файл **атомарно** с новым `version` (см. `authz_state`).

#### B. `servers` — реестр флота + наблюдаемость applied-authz (поля LKG)

```sql
CREATE TABLE servers (
    id                    INTEGER  PRIMARY KEY AUTOINCREMENT,
    name                  TEXT     NOT NULL,
    host                  TEXT     NOT NULL,
    port                  INTEGER  NOT NULL DEFAULT 443,
    api_base              TEXT     NULL,                    -- база для /health и /internal/authz/update (вариант B)
    status                TEXT     NOT NULL DEFAULT 'active'-- active / maintenance / offline
                          CHECK(status IN ('active','maintenance','offline')),
    priority              INTEGER  NOT NULL DEFAULT 100,    -- для IssuancePolicy (выбор сервера)
    -- === authz/health-поля (следствие решения last-known-good, 5.2) ===
    fail_mode             TEXT     NOT NULL DEFAULT 'lkg'   -- ОЖИДАЕМЫЙ режим Gate: lkg / open / closed
                          CHECK(fail_mode IN ('lkg','open','closed')),
    expected_authz_version INTEGER NOT NULL DEFAULT 0,      -- последняя version, которую панель ОТПРАВИЛА на этот сервер
    applied_authz_version  INTEGER NULL,                    -- version, которую сервер РЕАЛЬНО применил (из /health)
    lkg_valid_at          DATETIME NULL,                    -- когда Gate в последний раз успешно загрузил валидный файл
    load_errors           INTEGER  NOT NULL DEFAULT 0,      -- счётчик ошибок чтения/парсинга authz.json на сервере
    last_health_at        DATETIME NULL,                    -- момент последнего успешного health-poll
    notes                 TEXT     NULL,
    created_at            DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_servers_status ON servers(status);
```

Зачем authz/health-поля (прямо из Решения 4 / 5.2):
- При `fail_mode: lkg` сервер может легитимно работать по **устаревшему-но-валидному** allowlist. Поэтому «жив ли сервер» уже недостаточно — нужно видеть **свежесть применённого**: `applied_authz_version` vs `expected_authz_version`, возраст `lkg_valid_at`, `load_errors`.
- Расхождение `applied < expected` или `load_errors > 0` или устаревший `lkg_valid_at` → сервер в блоке «Требуют внимания» на дашборде ([[OlcPanel_06_UI_Карта_экранов]]) + алерт ([[OlcPanel_Анализ_Marzban_3xui]], 2.4/3.6).
- На single-server (MVP) есть одна строка `servers` (дефолтный сервер); поля заполняет локальный self-poll. Это **тот же контракт**, что и для флота — поэтому закладывается с Этапа 1.

#### C. `authz_state` — монотонная версия allowlist (источник `version` в `authz.json`)

```sql
CREATE TABLE authz_state (
    id            INTEGER  PRIMARY KEY DEFAULT 1 CHECK(id = 1), -- singleton (на single-server)
    version       INTEGER  NOT NULL DEFAULT 0,   -- МОНОТОННО ВОЗРАСТАЕТ при каждой записи authz.json
    updated_at    DATETIME NOT NULL DEFAULT (datetime('now')),
    allow_count   INTEGER  NOT NULL DEFAULT 0,   -- сколько device_id в последнем срезе (для дашборда/диагностики)
    last_writer   TEXT     NULL                  -- что инициировало: scheduler / api / manual / coordinator
);
INSERT OR IGNORE INTO authz_state (id, version) VALUES (1, 0);
```

- `authz_writer` перед записью делает `version = version + 1` (в одной транзакции) и кладёт это значение в payload. **Монотонность критична**: на варианте B сервер отвергает payload со `version ≤ applied`, не сбрасывая LKG (4.5 п.2 в [[OlcPanel_Анализ_Marzban_3xui]]).
- На multi-server `authz_state` становится **per-server** (ключ `server_id` вместо singleton) — версия у каждого сервера своя, т.к. срезы разные. Закладывается переходом `id`→`server_id` без слома (см. 2.1.4).
- Поле дублирует смысл `servers.expected_authz_version`: `authz_state.version` — «что панель сгенерировала глобально/для сервера», `servers.expected_authz_version` — «что отправлено на конкретный сервер», `servers.applied_authz_version` — «что сервер подтвердил». Три величины разнесены сознательно для диагностики доставки (split-brain, 5.3).

#### D. Схема файла `authz.json` (контракт панель→сервер)

```jsonc
{
  "version":    42,                         // = authz_state.version на момент записи; монотонна
  "updated_at": "2026-06-14T08:51:00Z",     // ISO8601 UTC
  "mode":       "allowlist",                // allowlist / denylist / off (из server.yaml authz.mode)
  "allow":      ["hwid-aaa", "hwid-bbb"],   // активные device_id (is_active && подписка active)
  "deny":       []                          // явные запреты (опционально; приоритет над allow)
}
```

Требования к записи (из 4.5 / 5.1 / 5.2, **обязательны с Этапа 1**):
1. **Атомарность:** `write tmp → fsync → os.rename`. Никаких дописываний на месте.
2. **Монотонный `version`** (из `authz_state`).
3. **Сдвиг mtime** гарантирован (`Gate` сравнивает `mtime.Equal`; при равной гранулярности ФС — явный `os.utime`/дедуп частых записей).
4. `Gate` валидирует схему и **отвергает откат version**, не сбрасывая LKG; при ошибке — `load_errors++` + алерт.

### 2.1.4 Семантика `server_id` и переход single→multi (детально)

`server_id` живёт в **двух** таблицах осознанно (раздел 4.6 [[OlcPanel_Анализ_Marzban_3xui]]):

- **`subscriptions.server_id`** — «на каком сервере *продан* доступ» (коммерческая привязка). Ставит `IssuancePolicy` при выдаче/продлении.
- **`user_devices.server_id`** — «на какой сервер *доставлять* allow для этого устройства» (рабочий ключ координатора). Обычно = подписке, но отделено ради: (а) нескольких устройств у клиента; (б) миграции устройства между серверами (меняется именно оно); (в) пересчёт среза идёт по нему.

Правило целостности: для активного устройства в multi-server-режиме `user_devices.server_id` **обязан** быть заполнен; FK на `servers.id` — мягкий (`ON DELETE SET NULL`) на переходный период.

Переход (без слома схемы):
1. **Этап 1:** `servers` (одна дефолтная строка), `server_id` nullable в `subscriptions`/`user_devices`, `authz_state` singleton. Поведение = single-server.
2. **Этап 2:** backfill `server_id` существующих записей (идемпотентный скрипт + dry-run + отчёт расхождений, 5.5); `authz_state` мигрирует в per-server; включается health-poll, заполняющий `applied_authz_version`/`lkg_valid_at`/`load_errors`.
3. **Этап 3:** миграция устройства между серверами (порядок «сначала добавить на новый, потом убрать со старого» — fail-safe для платящего, 4.7).

> Координатор/`authz_writer`/`Gate` детально описаны в [[OlcPanel_03_Архитектура_сервисов]]; доставка и health — в [[OlcPanel_07_Инфраструктура]]; угрозы устаревания — в [[OlcPanel_08_Безопасность]].
                        │ created_at      │
                        └─────────────────┘

┌──────────────────────┐     ┌───────────────────────┐
│    traffic_daily     │     │   traffic_snapshots   │
├──────────────────────┤     ├───────────────────────┤
│ id (PK)              │     │ id (PK)               │
│ client_id (FK)       │     │ client_id (FK)        │
│ date                 │     │ captured_at           │
│ bytes_in             │     │ bytes_in_total        │
│ bytes_out            │     │ bytes_out_total       │
│ bytes_total          │     └───────────────────────┘
│ created_at           │
└──────────────────────┘

┌─────────────────────┐     ┌─────────────────────┐
│   server_settings   │     │     audit_log       │
├─────────────────────┤     ├─────────────────────┤
│ id (PK)             │     │ id (PK)             │
│ jitsi_server        │     │ admin_id (FK)       │
│ default_room_prefix │     │ action              │
│ region_label        │     │ entity_type         │
│ transport           │     │ entity_id           │
│ dns_server          │     │ details             │
│ server_yaml_path    │     │ ip_address          │
│ binary_path         │     │ created_at          │
│ service_name        │     └─────────────────────┘
│ warn_days_before    │
│ updated_at          │
└─────────────────────┘
```

---

## 2.2 Детальное описание таблиц

### 2.2.1 Таблица `admins`

**Назначение:** Хранит учётные данные администраторов панели. В v1.0 всегда одна запись.

```sql
CREATE TABLE admins (
    id              INTEGER     PRIMARY KEY AUTOINCREMENT,
    username        TEXT        NOT NULL UNIQUE,
    password_hash   TEXT        NOT NULL,           -- bcrypt hash, cost=12
    created_at      DATETIME    NOT NULL DEFAULT (datetime('now')),
    last_login      DATETIME    NULL,               -- обновляется при каждом входе
    is_active       BOOLEAN     NOT NULL DEFAULT 1  -- для будущего мультиадмина
);
```

**Описание полей:**

| Поле | Тип | Nullable | Описание |
|------|-----|----------|----------|
| `id` | INTEGER | NO | Первичный ключ, автоинкремент |
| `username` | TEXT | NO | Логин администратора. Уникальный. Минимум 3 символа. |
| `password_hash` | TEXT | NO | bcrypt-хеш пароля. Никогда не хранится в открытом виде. cost=12 (≈250 мс на проверку — защита от брутфорса) |
| `created_at` | DATETIME | NO | Дата и время создания аккаунта. UTC. |
| `last_login` | DATETIME | YES | Последний успешный вход. NULL если никогда не входил. |
| `is_active` | BOOLEAN | NO | Флаг активности. При `0` — логин запрещён. Зарезервировано для будущего. |

**Индексы:**
```sql
-- Уникальность username гарантирует UNIQUE constraint выше
-- Дополнительных индексов не нужно (1–3 строки)
```

**Пример данных:**
```
id=1, username="admin", password_hash="$2b$12$K8Qz...", created_at="2026-06-10 12:00:00", last_login="2026-06-10 14:32:11", is_active=1
```

**SQLAlchemy модель:**
```python
class Admin(Base):
    __tablename__ = "admins"

    id            = Column(Integer, primary_key=True, autoincrement=True)
    username      = Column(String(64), nullable=False, unique=True)
    password_hash = Column(String(256), nullable=False)
    created_at    = Column(DateTime, nullable=False, default=func.now())
    last_login    = Column(DateTime, nullable=True)
    is_active     = Column(Boolean, nullable=False, default=True)
```

---

### 2.2.2 Таблица `plans`

**Назначение:** Тарифные планы. Определяют стоимость, длительность и лимит трафика подписки.

```sql
CREATE TABLE plans (
    id            INTEGER     PRIMARY KEY AUTOINCREMENT,
    name          TEXT        NOT NULL UNIQUE,       -- "Trial", "Базовый", "Стандарт", "Безлимит"
    description   TEXT        NULL,                  -- описание для администратора
    price         INTEGER     NOT NULL DEFAULT 0,    -- цена в рублях (копейки не нужны)
    duration_days INTEGER     NOT NULL DEFAULT 30,   -- длительность подписки в днях
    traffic_gb    INTEGER     NOT NULL DEFAULT 0,    -- лимит трафика, ГБ. 0 = безлимит
    is_active     BOOLEAN     NOT NULL DEFAULT 1,    -- можно ли создавать новые подписки
    sort_order    INTEGER     NOT NULL DEFAULT 0,    -- порядок отображения в UI
    created_at    DATETIME    NOT NULL DEFAULT (datetime('now')),
    updated_at    DATETIME    NOT NULL DEFAULT (datetime('now'))
);
```

**Описание полей:**

| Поле | Тип | Nullable | Описание |
|------|-----|----------|----------|
| `id` | INTEGER | NO | Первичный ключ |
| `name` | TEXT | NO | Отображаемое название тарифа. Уникальное. |
| `description` | TEXT | YES | Произвольное описание для администратора |
| `price` | INTEGER | NO | Цена в рублях. `0` = бесплатно (Trial). |
| `duration_days` | INTEGER | NO | Количество дней подписки. Например, `30` = 1 месяц. |
| `traffic_gb` | INTEGER | NO | Лимит трафика в гигабайтах. `0` = безлимитный трафик. |
| `is_active` | BOOLEAN | NO | `1` = тариф доступен для выбора. `0` = архивирован (существующие подписки не затрагиваются). |
| `sort_order` | INTEGER | NO | Порядок сортировки в списке тарифов. Меньше = выше. |
| `created_at` | DATETIME | NO | Дата создания тарифа |
| `updated_at` | DATETIME | NO | Дата последнего изменения. Обновляется триггером или ORM. |

**Предустановленные данные (seed data):**

```sql
INSERT INTO plans (name, description, price, duration_days, traffic_gb, is_active, sort_order) VALUES
    ('Trial',    'Пробный период. Бесплатно, 7 дней, 5 ГБ',          0,   7,  5,   1, 0),
    ('Базовый',  'Для лёгкого использования. 30 дней, 30 ГБ',       300,  30, 30,  1, 1),
    ('Стандарт', 'Оптимальный выбор. 30 дней, 100 ГБ',              500,  30, 100, 1, 2),
    ('Безлимит', 'Для активных пользователей. 30 дней, без лимита', 700,  30, 0,   1, 3);
```

**SQLAlchemy модель:**
```python
class Plan(Base):
    __tablename__ = "plans"

    id            = Column(Integer, primary_key=True, autoincrement=True)
    name          = Column(String(64), nullable=False, unique=True)
    description   = Column(Text, nullable=True)
    price         = Column(Integer, nullable=False, default=0)
    duration_days = Column(Integer, nullable=False, default=30)
    traffic_gb    = Column(Integer, nullable=False, default=0)
    is_active     = Column(Boolean, nullable=False, default=True)
    sort_order    = Column(Integer, nullable=False, default=0)
    created_at    = Column(DateTime, nullable=False, default=func.now())
    updated_at    = Column(DateTime, nullable=False, default=func.now(), onupdate=func.now())

    # Relations
    subscriptions = relationship("Subscription", back_populates="plan")
```

---

### 2.2.3 Таблица `clients`

**Назначение:** Основная таблица клиентов VPN-сервиса.

```sql
CREATE TABLE clients (
    id          INTEGER     PRIMARY KEY AUTOINCREMENT,
    name        TEXT        NOT NULL,               -- отображаемое имя, например "Антон"
    telegram    TEXT        NULL,                   -- @username или числовой ID
    phone       TEXT        NULL,                   -- телефон (необязательно)
    notes       TEXT        NULL,                   -- заметки администратора
    status      TEXT        NOT NULL DEFAULT 'active'
                            CHECK(status IN ('active', 'suspended', 'expired', 'deleted')),
    created_at  DATETIME    NOT NULL DEFAULT (datetime('now')),
    updated_at  DATETIME    NOT NULL DEFAULT (datetime('now')),
    deleted_at  DATETIME    NULL                    -- soft delete: дата удаления
);

-- Индексы
CREATE INDEX idx_clients_status     ON clients(status);
CREATE INDEX idx_clients_created_at ON clients(created_at);
CREATE INDEX idx_clients_telegram   ON clients(telegram);
```

**Описание полей:**

| Поле | Тип | Nullable | Описание |
|------|-----|----------|----------|
| `id` | INTEGER | NO | Первичный ключ |
| `name` | TEXT | NO | Имя клиента для идентификации. Не уникальное (могут быть два Антона). Минимум 1 символ. |
| `telegram` | TEXT | YES | Telegram username (`@username`) или числовой ID. Для контакта с клиентом. |
| `phone` | TEXT | YES | Номер телефона. Хранится как строка (с кодом страны, пробелами — не форматируем). |
| `notes` | TEXT | YES | Произвольные заметки администратора. Например: "Друг Миши, Москва, платит криптой" |
| `status` | TEXT | NO | Статус клиента. Enum: `active` / `suspended` / `expired` / `deleted`. |
| `created_at` | DATETIME | NO | Дата регистрации клиента в системе |
| `updated_at` | DATETIME | NO | Дата последнего изменения любого поля |
| `deleted_at` | DATETIME | YES | Дата мягкого удаления. `NULL` = не удалён. Soft delete сохраняет данные 90 дней. |

**Статусы клиента — state machine:**

```
                  ┌─────────────────────────────────────────┐
                  │                                         │
   [Создан] ──▶ active ──▶ suspended ──▶ [продление] ──▶ active
                  │            ▲
                  │            │ [продление]
                  ▼            │
               expired ────────┘
                  │
                  ▼ [ручное удаление]
               deleted
```

| Статус | Описание | VPN работает? | Действия |
|--------|----------|:---:|---------|
| `active` | Подписка активна | ✅ Да | Продлить, Отключить, Удалить |
| `suspended` | Вручную приостановлен или отключён по лимиту | ❌ Нет | Восстановить, Продлить, Удалить |
| `expired` | Подписка истекла (авто) | ❌ Нет | Продлить (→ active), Удалить |
| `deleted` | Мягко удалён | ❌ Нет | Восстановить (90 дней), Удалить навсегда |

**Примечание:** статус клиента (`clients.status`) дублирует логику из `subscriptions.status`. Они синхронизируются автоматически через APScheduler. `clients.status` используется для быстрой фильтрации без JOIN.

**SQLAlchemy модель:**
```python
class ClientStatus(str, enum.Enum):
    ACTIVE    = "active"
    SUSPENDED = "suspended"
    EXPIRED   = "expired"
    DELETED   = "deleted"

class Client(Base):
    __tablename__ = "clients"

    id         = Column(Integer, primary_key=True, autoincrement=True)
    name       = Column(String(128), nullable=False)
    telegram   = Column(String(128), nullable=True, index=True)
    phone      = Column(String(32), nullable=True)
    notes      = Column(Text, nullable=True)
    status     = Column(SQLEnum(ClientStatus), nullable=False, default=ClientStatus.ACTIVE, index=True)
    created_at = Column(DateTime, nullable=False, default=func.now())
    updated_at = Column(DateTime, nullable=False, default=func.now(), onupdate=func.now())
    deleted_at = Column(DateTime, nullable=True)

    # Relations
    configs       = relationship("Config", back_populates="client",
                                 order_by="Config.created_at.desc()")
    subscriptions = relationship("Subscription", back_populates="client",
                                 order_by="Subscription.created_at.desc()")
    payments      = relationship("Payment", back_populates="client")
    traffic_daily = relationship("TrafficDaily", back_populates="client")

    @property
    def active_config(self):
        """Возвращает текущий активный конфиг клиента."""
        return next((c for c in self.configs if c.status == "active"), None)

    @property
    def active_subscription(self):
        """Возвращает текущую активную подписку."""
        return next((s for s in self.subscriptions if s.status == "active"), None)
```

**Пример данных:**
```
id=1, name="Антон", telegram="@anton_vpn", phone="+79161234567", notes="Друг Миши",
      status="active", created_at="2026-06-01 10:00:00", updated_at="2026-06-01 10:00:00",
      deleted_at=NULL

id=2, name="Мария", telegram="@masha_m", phone=NULL, notes="Из Питера, платит каждые 3 месяца",
      status="expired", created_at="2026-03-15 09:30:00", updated_at="2026-06-10 01:00:00",
      deleted_at=NULL
```

---

### 2.2.4 Таблица `configs` — 🟠 LEGACY (не источник доступа в v2.0)

> ⚠️ **LEGACY.** В v2.0 ключ и комната **общие** (из `server.yaml`), а доступ управляется через `user_devices.device_id` + `authz.json` (Решение 1, [[OlcPanel_00_Введение]]). Таблица `configs` сохранена для **истории/совместимости** со старыми записями и аудита смен; **новые клиенты не получают пер-клиентский `crypto_key`/комнату**. Поле `uri` теперь означает ссылку подписки на общую комнату, а не уникальный per-client ключ. Не используйте `configs` как allowlist — источник доступа только `user_devices`.

**Назначение (исторически):** OlcRTC-конфигурации клиентов. Хранит историю всех конфигов (текущий + отозванные).

```sql
CREATE TABLE configs (
    id            INTEGER     PRIMARY KEY AUTOINCREMENT,
    client_id     INTEGER     NOT NULL REFERENCES clients(id) ON DELETE RESTRICT,
    crypto_key    TEXT        NOT NULL,    -- 64-символьный hex (256-bit), уникальный
    room_name     TEXT        NOT NULL,    -- короткое имя комнаты, например "olcrtc-a1b2c3d4"
    room_url      TEXT        NOT NULL,    -- полный URL комнаты, например "https://meet1.arbitr.ru/olcrtc-a1b2c3d4"
    jitsi_server  TEXT        NOT NULL,    -- хост Jitsi, например "meet1.arbitr.ru"
    transport     TEXT        NOT NULL DEFAULT 'datachannel',  -- тип транспорта OlcRTC
    region_label  TEXT        NULL,        -- метка региона, например "EU/Amsterdam"
    uri           TEXT        NOT NULL,    -- полный OlcRTC URI (генерируется при сохранении)
    status        TEXT        NOT NULL DEFAULT 'active'
                              CHECK(status IN ('active', 'revoked')),
    created_at    DATETIME    NOT NULL DEFAULT (datetime('now')),
    revoked_at    DATETIME    NULL,        -- время отзыва, NULL если активен
    revoke_reason TEXT        NULL         -- причина отзыва (опционально)
);

-- Индексы
CREATE UNIQUE INDEX idx_configs_crypto_key ON configs(crypto_key);  -- ключ уникален глобально
CREATE INDEX idx_configs_client_id         ON configs(client_id);
CREATE INDEX idx_configs_status            ON configs(status);
```

**Описание полей:**

| Поле | Тип | Nullable | Описание |
|------|-----|----------|----------|
| `id` | INTEGER | NO | Первичный ключ |
| `client_id` | INTEGER | NO | FK на `clients.id`. При удалении клиента — запрет (ON DELETE RESTRICT). |
| `crypto_key` | TEXT | NO | 64-символьный hex-строка (32 байта). Глобально уникальна. Генерируется через `secrets.token_hex(32)`. |
| `room_name` | TEXT | NO | Короткое имя комнаты. Формат: `olcrtc-{8_символов_uuid}`. Например: `olcrtc-a1b2c3d4`. |
| `room_url` | TEXT | NO | Полный URL комнаты. `{jitsi_server}/{room_name}`. Например: `https://meet1.arbitr.ru/olcrtc-a1b2c3d4`. |
| `jitsi_server` | TEXT | NO | Хост Jitsi-сервера. Берётся из `server_settings.jitsi_server` на момент создания. |
| `transport` | TEXT | NO | Транспортный протокол. `datachannel` (WebRTC SCTP) — единственный стабильный вариант. |
| `region_label` | TEXT | YES | Метка для URI. Например `EU/Amsterdam`. Косметическая, не влияет на подключение. |
| `uri` | TEXT | NO | Полный OlcRTC URI. Вычисляется при создании и кэшируется. Формат описан в разделе 5 (Бизнес-логика). |
| `status` | TEXT | NO | `active` = конфиг используется. `revoked` = конфиг отозван, использовать нельзя. |
| `created_at` | DATETIME | NO | Дата создания конфига |
| `revoked_at` | DATETIME | YES | Дата отзыва. NULL если активен. |
| `revoke_reason` | TEXT | YES | Причина отзыва. Например: "Клиент поделился конфигом". |

**Инварианты (бизнес-правила):**

1. У каждого клиента может быть **не более одного** активного конфига (status='active')
2. При смене конфига: текущий → `revoked`, создаётся новый `active`
3. `crypto_key` глобально уникален (уникальный индекс)
4. `uri` не хранится в server_settings — берётся из конфига клиента

**SQLAlchemy модель:**
```python
class ConfigStatus(str, enum.Enum):
    ACTIVE  = "active"
    REVOKED = "revoked"

class Config(Base):
    __tablename__ = "configs"

    id            = Column(Integer, primary_key=True, autoincrement=True)
    client_id     = Column(Integer, ForeignKey("clients.id", ondelete="RESTRICT"), nullable=False, index=True)
    crypto_key    = Column(String(64), nullable=False, unique=True)
    room_name     = Column(String(64), nullable=False)
    room_url      = Column(String(256), nullable=False)
    jitsi_server  = Column(String(128), nullable=False)
    transport     = Column(String(32), nullable=False, default="datachannel")
    region_label  = Column(String(64), nullable=True)
    uri           = Column(Text, nullable=False)
    status        = Column(SQLEnum(ConfigStatus), nullable=False, default=ConfigStatus.ACTIVE, index=True)
    created_at    = Column(DateTime, nullable=False, default=func.now())
    revoked_at    = Column(DateTime, nullable=True)
    revoke_reason = Column(Text, nullable=True)

    # Relations
    client = relationship("Client", back_populates="configs")

    def to_yaml(self) -> str:
        """Генерирует YAML-конфиг для OlcBox."""
        return f"""mode: cnc
auth:
  provider: jitsi
room:
  id: "{self.room_url}"
crypto:
  key: "{self.crypto_key}"
net:
  transport: {self.transport}
  dns: "8.8.8.8:53"
socks:
  host: "127.0.0.1"
  port: 8808
data: data
"""
```

**Пример данных:**
```
id=1, client_id=1, crypto_key="a3f8b1c2d4e5...(64 hex)", room_name="olcrtc-a1b2c3d4",
      room_url="https://meet1.arbitr.ru/olcrtc-a1b2c3d4",
      jitsi_server="meet1.arbitr.ru", transport="datachannel", region_label="EU/Amsterdam",
      uri="olcrtc://jitsi?datachannel@https://meet1.arbitr.ru/olcrtc-a1b2c3d4#a3f8b1...#EU/Amsterdam",
      status="active", created_at="2026-06-01 10:00:05", revoked_at=NULL, revoke_reason=NULL

id=2, client_id=1, crypto_key="9f1e2d3c4b5a...(64 hex)", room_name="olcrtc-b5c6d7e8",
      ...
      status="revoked", created_at="2026-05-01 09:00:00", revoked_at="2026-06-01 10:00:05",
      revoke_reason="Клиент поделился ссылкой"
```

---

### 2.2.5 Таблица `subscriptions`

**Назначение:** Подписки клиентов. Связывает клиента с тарифом и периодом действия.

```sql
CREATE TABLE subscriptions (
    id               INTEGER     PRIMARY KEY AUTOINCREMENT,
    client_id        INTEGER     NOT NULL REFERENCES clients(id) ON DELETE RESTRICT,
    plan_id          INTEGER     NOT NULL REFERENCES plans(id) ON DELETE RESTRICT,
    status           TEXT        NOT NULL DEFAULT 'active'
                                 CHECK(status IN ('active', 'expired', 'suspended', 'cancelled')),
    started_at       DATETIME    NOT NULL,               -- начало периода
    expires_at       DATETIME    NOT NULL,               -- конец периода
    traffic_limit_mb INTEGER     NOT NULL DEFAULT 0,     -- лимит трафика в МБ (0 = безлимит), копируется из плана
    traffic_used_mb  INTEGER     NOT NULL DEFAULT 0,     -- использовано трафика в МБ (обновляется APScheduler)
    created_at       DATETIME    NOT NULL DEFAULT (datetime('now')),
    updated_at       DATETIME    NOT NULL DEFAULT (datetime('now'))
);

-- Индексы
CREATE INDEX idx_subscriptions_client_id  ON subscriptions(client_id);
CREATE INDEX idx_subscriptions_status     ON subscriptions(status);
CREATE INDEX idx_subscriptions_expires_at ON subscriptions(expires_at);
-- Для быстрого поиска истекающих подписок (APScheduler каждый час)
CREATE INDEX idx_subscriptions_active_expires ON subscriptions(status, expires_at)
    WHERE status = 'active';
```

**Описание полей:**

| Поле | Тип | Nullable | Описание |
|------|-----|----------|----------|
| `id` | INTEGER | NO | Первичный ключ |
| `client_id` | INTEGER | NO | FK на `clients.id` |
| `plan_id` | INTEGER | NO | FK на `plans.id`. Хранится для истории даже если план изменился. |
| `status` | TEXT | NO | Статус подписки. `active` / `expired` / `suspended` / `cancelled`. |
| `started_at` | DATETIME | NO | Начало периода подписки. Обычно = дата продления или создания. |
| `expires_at` | DATETIME | NO | Конец периода. `started_at + plan.duration_days`. |
| `traffic_limit_mb` | INTEGER | NO | Лимит трафика в МБ. Копируется из `plan.traffic_gb * 1024` при создании. `0` = безлимит. Хранится здесь для независимости от изменений плана. |
| `traffic_used_mb` | INTEGER | NO | Использовано трафика в МБ. Обновляется APScheduler каждые 5 минут из `traffic_snapshots`. Сбрасывается в `0` при продлении. |
| `created_at` | DATETIME | NO | Дата создания записи |
| `updated_at` | DATETIME | NO | Дата последнего обновления |

**Инварианты:**

1. У клиента может быть **не более одной** подписки со `status='active'`
2. При продлении: текущая подписка не изменяется — создаётся **новая** запись
3. `expires_at` новой подписки = `MAX(now(), текущая expires_at) + duration_days`
   - Это значит: продление до истечения — добавляет дни к текущей дате окончания
   - Продление после истечения — добавляет дни к сегодняшней дате
4. `traffic_limit_mb` и `traffic_used_mb` хранятся в МБ для точности (не ГБ)

**Статусы подписки:**

| Статус | Описание |
|--------|----------|
| `active` | Подписка действует, трафик считается |
| `expired` | Срок истёк (expires_at < now()) — ставит APScheduler |
| `suspended` | Приостановлена вручную или по лимиту трафика |
| `cancelled` | Отменена администратором (зарезервировано) |

**SQLAlchemy модель:**
```python
class SubscriptionStatus(str, enum.Enum):
    ACTIVE    = "active"
    EXPIRED   = "expired"
    SUSPENDED = "suspended"
    CANCELLED = "cancelled"

class Subscription(Base):
    __tablename__ = "subscriptions"

    id               = Column(Integer, primary_key=True, autoincrement=True)
    client_id        = Column(Integer, ForeignKey("clients.id", ondelete="RESTRICT"), nullable=False, index=True)
    plan_id          = Column(Integer, ForeignKey("plans.id", ondelete="RESTRICT"), nullable=False)
    status           = Column(SQLEnum(SubscriptionStatus), nullable=False,
                              default=SubscriptionStatus.ACTIVE, index=True)
    started_at       = Column(DateTime, nullable=False)
    expires_at       = Column(DateTime, nullable=False, index=True)
    traffic_limit_mb = Column(Integer, nullable=False, default=0)
    traffic_used_mb  = Column(Integer, nullable=False, default=0)
    created_at       = Column(DateTime, nullable=False, default=func.now())
    updated_at       = Column(DateTime, nullable=False, default=func.now(), onupdate=func.now())

    # Relations
    client   = relationship("Client", back_populates="subscriptions")
    plan     = relationship("Plan", back_populates="subscriptions")
    payments = relationship("Payment", back_populates="subscription")

    @property
    def traffic_used_percent(self) -> float:
        """Процент использованного трафика. 0.0 если безлимит."""
        if self.traffic_limit_mb == 0:
            return 0.0
        return round(self.traffic_used_mb / self.traffic_limit_mb * 100, 1)

    @property
    def days_left(self) -> int:
        """Дней до окончания подписки. Отрицательное если истекла."""
        delta = self.expires_at - datetime.utcnow()
        return delta.days

    @property
    def is_traffic_exceeded(self) -> bool:
        """Превышен ли лимит трафика."""
        if self.traffic_limit_mb == 0:
            return False
        return self.traffic_used_mb >= self.traffic_limit_mb
```

---

### 2.2.6 Таблица `payments`

**Назначение:** История платежей клиентов. Ручной учёт оплат.

```sql
CREATE TABLE payments (
    id              INTEGER     PRIMARY KEY AUTOINCREMENT,
    client_id       INTEGER     NOT NULL REFERENCES clients(id) ON DELETE RESTRICT,
    subscription_id INTEGER     NULL REFERENCES subscriptions(id) ON DELETE SET NULL,
    amount          INTEGER     NOT NULL DEFAULT 0,   -- сумма в рублях
    currency        TEXT        NOT NULL DEFAULT 'RUB',
    method          TEXT        NOT NULL DEFAULT 'card'
                                CHECK(method IN ('card', 'crypto', 'cash', 'other')),
    period_days     INTEGER     NOT NULL DEFAULT 30,  -- за сколько дней оплата
    note            TEXT        NULL,                 -- произвольная заметка
    created_at      DATETIME    NOT NULL DEFAULT (datetime('now'))
);

-- Индексы
CREATE INDEX idx_payments_client_id ON payments(client_id);
CREATE INDEX idx_payments_created_at ON payments(created_at);
```

**Описание полей:**

| Поле | Тип | Nullable | Описание |
|------|-----|----------|----------|
| `id` | INTEGER | NO | Первичный ключ |
| `client_id` | INTEGER | NO | FK на `clients.id` |
| `subscription_id` | INTEGER | YES | FK на `subscriptions.id`. Может быть NULL если запись создана вручную без привязки к конкретной подписке. |
| `amount` | INTEGER | NO | Сумма оплаты в рублях. `0` если Trial или бесплатно. |
| `currency` | TEXT | NO | Валюта. В v1.0 всегда `RUB`. Зарезервировано для крипты (USDT и т.д.). |
| `method` | TEXT | NO | Способ оплаты: `card` / `crypto` / `cash` / `other`. |
| `period_days` | INTEGER | NO | За сколько дней произведена оплата. Обычно 30, 60 или 90. |
| `note` | TEXT | YES | Произвольная заметка. Например: "Перевод от 10.06, проверено". |
| `created_at` | DATETIME | NO | Дата записи платежа |

**SQLAlchemy модель:**
```python
class PaymentMethod(str, enum.Enum):
    CARD   = "card"
    CRYPTO = "crypto"
    CASH   = "cash"
    OTHER  = "other"

class Payment(Base):
    __tablename__ = "payments"

    id              = Column(Integer, primary_key=True, autoincrement=True)
    client_id       = Column(Integer, ForeignKey("clients.id", ondelete="RESTRICT"), nullable=False, index=True)
    subscription_id = Column(Integer, ForeignKey("subscriptions.id", ondelete="SET NULL"), nullable=True)
    amount          = Column(Integer, nullable=False, default=0)
    currency        = Column(String(8), nullable=False, default="RUB")
    method          = Column(SQLEnum(PaymentMethod), nullable=False, default=PaymentMethod.CARD)
    period_days     = Column(Integer, nullable=False, default=30)
    note            = Column(Text, nullable=True)
    created_at      = Column(DateTime, nullable=False, default=func.now())

    # Relations
    client       = relationship("Client", back_populates="payments")
    subscription = relationship("Subscription", back_populates="payments")
```

---

### 2.2.7 Таблица `traffic_daily` — 🟤 DORMANT (учёт не настроен)

> ⚠️ **DORMANT.** Пер-клиентский трафик-аккаунтинг в v2.0 **не реализуется** (Решение 5): WebRTC/Jitsi-туннель не отдаёт пер-клиентскую статистику. Схема оставлена «спящей» под возможную будущую серверную телеметрию (Этап 3, 3.7 [[OlcPanel_Анализ_Marzban_3xui]]). В UI трафик показывается как «лимит N GB · учёт не настроен». Джоба заполнения **не запускается**.

**Назначение (под будущее):** Агрегированная суточная статистика трафика по каждому клиенту.

```sql
CREATE TABLE traffic_daily (
    id           INTEGER     PRIMARY KEY AUTOINCREMENT,
    client_id    INTEGER     NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    date         DATE        NOT NULL,               -- только дата, без времени (YYYY-MM-DD)
    bytes_in     INTEGER     NOT NULL DEFAULT 0,     -- входящий трафик за день, байты
    bytes_out    INTEGER     NOT NULL DEFAULT 0,     -- исходящий трафик за день, байты
    bytes_total  INTEGER     NOT NULL DEFAULT 0,     -- суммарный трафик (bytes_in + bytes_out)
    created_at   DATETIME    NOT NULL DEFAULT (datetime('now'))
);

-- Уникальность: один клиент = одна запись на дату
CREATE UNIQUE INDEX idx_traffic_daily_client_date ON traffic_daily(client_id, date);
CREATE INDEX idx_traffic_daily_date ON traffic_daily(date);
```

**Описание полей:**

| Поле | Тип | Nullable | Описание |
|------|-----|----------|----------|
| `id` | INTEGER | NO | Первичный ключ |
| `client_id` | INTEGER | NO | FK на `clients.id`. CASCADE DELETE: при удалении клиента статистика тоже удаляется. |
| `date` | DATE | NO | Дата (без времени). Уникальна в паре с `client_id`. |
| `bytes_in` | INTEGER | NO | Входящий трафик клиента за этот день в байтах |
| `bytes_out` | INTEGER | NO | Исходящий трафик клиента за этот день в байтах |
| `bytes_total` | INTEGER | NO | Сумма: `bytes_in + bytes_out`. Денормализовано для быстрого запроса. |
| `created_at` | DATETIME | NO | Дата создания/последнего обновления записи |

**Как заполняется:**

APScheduler каждые 5 минут читает `traffic_snapshots` (сырые данные iptables), вычисляет дельту с предыдущим снимком и прибавляет к записи за сегодня через `INSERT OR REPLACE`.

**SQLAlchemy модель:**
```python
class TrafficDaily(Base):
    __tablename__ = "traffic_daily"
    __table_args__ = (
        UniqueConstraint("client_id", "date", name="uq_traffic_daily_client_date"),
    )

    id          = Column(Integer, primary_key=True, autoincrement=True)
    client_id   = Column(Integer, ForeignKey("clients.id", ondelete="CASCADE"), nullable=False, index=True)
    date        = Column(Date, nullable=False, index=True)
    bytes_in    = Column(BigInteger, nullable=False, default=0)
    bytes_out   = Column(BigInteger, nullable=False, default=0)
    bytes_total = Column(BigInteger, nullable=False, default=0)
    created_at  = Column(DateTime, nullable=False, default=func.now())

    # Relations
    client = relationship("Client", back_populates="traffic_daily")
```

---

### 2.2.8 Таблица `traffic_snapshots` — 🟤 DORMANT (iptables-аккаунтинга нет)

> ⚠️ **DORMANT.** Зависела от iptables-MARK счётчиков, которых в модели «общая комната + WebRTC» нет. Не наполняется. Сохранена со схемой ради будущей телеметрии.

**Назначение (под будущее):** Сырые снимки счётчиков. Использовались бы для вычисления дельты трафика.

```sql
CREATE TABLE traffic_snapshots (
    id              INTEGER     PRIMARY KEY AUTOINCREMENT,
    client_id       INTEGER     NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    captured_at     DATETIME    NOT NULL DEFAULT (datetime('now')),
    bytes_in_total  INTEGER     NOT NULL DEFAULT 0,  -- накопленный входящий с iptables (монотонный счётчик)
    bytes_out_total INTEGER     NOT NULL DEFAULT 0   -- накопленный исходящий (монотонный счётчик)
);

-- Индексы
CREATE INDEX idx_traffic_snapshots_client_captured ON traffic_snapshots(client_id, captured_at);
-- Для быстрой очистки старых снимков
CREATE INDEX idx_traffic_snapshots_captured_at ON traffic_snapshots(captured_at);
```

**Описание полей:**

| Поле | Тип | Nullable | Описание |
|------|-----|----------|----------|
| `id` | INTEGER | NO | Первичный ключ |
| `client_id` | INTEGER | NO | FK на `clients.id` |
| `captured_at` | DATETIME | NO | Момент снятия снимка |
| `bytes_in_total` | INTEGER | NO | Монотонный счётчик входящих байт от iptables. Может только расти (сбрасывается при перезагрузке — обрабатывается в бизнес-логике). |
| `bytes_out_total` | INTEGER | NO | Монотонный счётчик исходящих байт |

**Ротация:** APScheduler удаляет снимки старше 48 часов ежедневно в 04:00 UTC.

**Принцип работы:**
```
Снимок T1: bytes_in_total = 1 500 000
Снимок T2: bytes_in_total = 2 300 000
Дельта за период T1→T2: 2 300 000 - 1 500 000 = 800 000 байт (≈ 0.76 МБ)
Эти 800 000 байт прибавляются к traffic_daily за сегодня.
```

**SQLAlchemy модель:**
```python
class TrafficSnapshot(Base):
    __tablename__ = "traffic_snapshots"

    id              = Column(Integer, primary_key=True, autoincrement=True)
    client_id       = Column(Integer, ForeignKey("clients.id", ondelete="CASCADE"), nullable=False)
    captured_at     = Column(DateTime, nullable=False, default=func.now(), index=True)
    bytes_in_total  = Column(BigInteger, nullable=False, default=0)
    bytes_out_total = Column(BigInteger, nullable=False, default=0)
```

---

### 2.2.9 Таблица `server_settings`

**Назначение:** Настройки OlcRTC-сервера. Одна строка (singleton-таблица).

```sql
CREATE TABLE server_settings (
    id                  INTEGER     PRIMARY KEY DEFAULT 1
                                    CHECK(id = 1),   -- гарантируем одну строку
    jitsi_server        TEXT        NOT NULL DEFAULT 'meet1.arbitr.ru',
    default_room_prefix TEXT        NOT NULL DEFAULT 'olcrtc',
    region_label        TEXT        NOT NULL DEFAULT 'EU/Amsterdam',
    transport           TEXT        NOT NULL DEFAULT 'datachannel',
    dns_server          TEXT        NOT NULL DEFAULT '8.8.8.8:53',
    socks_port          INTEGER     NOT NULL DEFAULT 8808,
    server_yaml_path    TEXT        NOT NULL DEFAULT '/root/olcrtc/server.yaml',
    binary_path         TEXT        NOT NULL DEFAULT '/root/olcrtc/build/olcrtc-linux-amd64',
    service_name        TEXT        NOT NULL DEFAULT 'olcrtc-server',
    warn_days_before    INTEGER     NOT NULL DEFAULT 3,   -- за сколько дней предупреждать об истечении
    -- === authz/Phase 1 (v2.0) ===
    authz_json_path     TEXT        NOT NULL DEFAULT '/opt/olcrtc-panel/data/authz.json', -- куда писать allowlist
    authz_mode          TEXT        NOT NULL DEFAULT 'off',  -- зеркало server.yaml authz.mode: off/allowlist/denylist
    authz_fail_mode     TEXT        NOT NULL DEFAULT 'lkg',  -- ОЖИДАЕМЫЙ fail_mode Gate: lkg(прод)/open/closed
    lkg_max_age_sec     INTEGER     NOT NULL DEFAULT 86400,  -- порог устаревания LKG (эскалация/алерт), 0=выкл
    updated_at          DATETIME    NOT NULL DEFAULT (datetime('now'))
);

-- Инициализация (один раз)
INSERT OR IGNORE INTO server_settings (id) VALUES (1);
```

> Поля `authz_*` зеркалят решение **last-known-good** (5.2) на уровне настроек: `authz_fail_mode='lkg'` — прод-дефолт, `lkg_max_age_sec` задаёт, когда устаревший allowlist эскалируется в алерт/`closed`. На multi-server эти поля переезжают в `servers` (см. 2.1.3-B `fail_mode`), а `server_settings` хранит глобальный дефолт.

**Описание полей:**

| Поле | Тип | Описание |
|------|-----|----------|
| `id` | INTEGER | Всегда `1`. CHECK constraint предотвращает вторую строку. |
| `jitsi_server` | TEXT | Текущий Jitsi-сервер. Используется при генерации новых конфигов. |
| `default_room_prefix` | TEXT | Префикс для имён комнат. Результат: `{prefix}-{uuid8}`. |
| `region_label` | TEXT | Метка региона для URI. Косметическая. |
| `transport` | TEXT | Тип транспорта OlcRTC. Сейчас всегда `datachannel`. |
| `dns_server` | TEXT | DNS-сервер для клиентов. Пишется в YAML-конфиг. |
| `socks_port` | INTEGER | Локальный SOCKS5-порт на клиентском устройстве. Стандарт: 8808. |
| `server_yaml_path` | TEXT | Путь к конфигу OlcRTC-сервера. Нужен для обновления при смене Jitsi. |
| `binary_path` | TEXT | Путь к бинарнику OlcRTC. Нужен для проверки статуса. |
| `service_name` | TEXT | Имя systemd-сервиса. Нужен для restart/status команд. |
| `warn_days_before` | INTEGER | За сколько дней до истечения подписки клиент попадает в "Требуют внимания". |
| `updated_at` | DATETIME | Дата последнего изменения настроек. |

**SQLAlchemy модель:**
```python
class ServerSettings(Base):
    __tablename__ = "server_settings"

    id                 = Column(Integer, primary_key=True, default=1)
    jitsi_server       = Column(String(256), nullable=False, default="meet1.arbitr.ru")
    default_room_prefix = Column(String(32), nullable=False, default="olcrtc")
    region_label       = Column(String(64), nullable=False, default="EU/Amsterdam")
    transport          = Column(String(32), nullable=False, default="datachannel")
    dns_server         = Column(String(64), nullable=False, default="8.8.8.8:53")
    socks_port         = Column(Integer, nullable=False, default=8808)
    server_yaml_path   = Column(String(256), nullable=False, default="/root/olcrtc/server.yaml")
    binary_path        = Column(String(256), nullable=False, default="/root/olcrtc/build/olcrtc-linux-amd64")
    service_name       = Column(String(64), nullable=False, default="olcrtc-server")
    warn_days_before   = Column(Integer, nullable=False, default=3)
    updated_at         = Column(DateTime, nullable=False, default=func.now(), onupdate=func.now())
```

---

### 2.2.10 Таблица `audit_log`

**Назначение:** Журнал всех действий администратора. Для аудита и истории изменений.

```sql
CREATE TABLE audit_log (
    id          INTEGER     PRIMARY KEY AUTOINCREMENT,
    admin_id    INTEGER     NULL REFERENCES admins(id) ON DELETE SET NULL,
    action      TEXT        NOT NULL,       -- например: "client.create", "config.revoke", "subscription.extend"
    entity_type TEXT        NULL,           -- например: "client", "config", "subscription"
    entity_id   INTEGER     NULL,           -- ID затронутой записи
    details     TEXT        NULL,           -- JSON с деталями (старые/новые значения)
    ip_address  TEXT        NULL,           -- IP администратора
    created_at  DATETIME    NOT NULL DEFAULT (datetime('now'))
);

-- Индексы
CREATE INDEX idx_audit_log_admin_id   ON audit_log(admin_id);
CREATE INDEX idx_audit_log_created_at ON audit_log(created_at);
CREATE INDEX idx_audit_log_entity     ON audit_log(entity_type, entity_id);
```

**Примеры записей:**

```
action="client.create",      entity_type="client",       entity_id=5,  details='{"name":"Антон","plan_id":3}'
action="config.revoke",      entity_type="config",        entity_id=3,  details='{"reason":"Поделился конфигом"}'
action="subscription.extend",entity_type="subscription",  entity_id=7,  details='{"days":30,"new_expires":"2026-08-10"}'
action="client.suspend",     entity_type="client",        entity_id=2,  details='{"reason":"manual"}'
action="auth.login",         entity_type=NULL,            entity_id=NULL,details='{"success":true}'
action="auth.login_failed",  entity_type=NULL,            entity_id=NULL,details='{"ip":"1.2.3.4","attempts":3}'
action="settings.update",    entity_type="server_settings",entity_id=1, details='{"jitsi_server":"meet2.example.com"}'
```

**SQLAlchemy модель:**
```python
class AuditLog(Base):
    __tablename__ = "audit_log"

    id          = Column(Integer, primary_key=True, autoincrement=True)
    admin_id    = Column(Integer, ForeignKey("admins.id", ondelete="SET NULL"), nullable=True, index=True)
    action      = Column(String(64), nullable=False)
    entity_type = Column(String(32), nullable=True)
    entity_id   = Column(Integer, nullable=True)
    details     = Column(Text, nullable=True)   # JSON string
    ip_address  = Column(String(64), nullable=True)
    created_at  = Column(DateTime, nullable=False, default=func.now(), index=True)
```

---

## 2.3 Миграции (Alembic)

### 2.3.1 Инициализация

```bash
# Создать директорию миграций
alembic init alembic

# В alembic.ini: sqlalchemy.url = sqlite:////opt/olcpanel/data/olcpanel.db

# Создать первую миграцию (все таблицы сразу)
alembic revision --autogenerate -m "initial_schema"

# Применить
alembic upgrade head
```

### 2.3.2 Структура директории миграций

```
alembic/
├── env.py               # конфигурация Alembic
├── script.py.mako       # шаблон
└── versions/
    ├── 001_initial_schema.py         # все таблицы
    ├── 002_seed_plans.py             # предустановленные тарифы
    └── 003_seed_server_settings.py   # начальные настройки сервера
```

### 2.3.3 Пример миграции с seed-данными

```python
# alembic/versions/002_seed_plans.py

from alembic import op
import sqlalchemy as sa

def upgrade():
    op.execute("""
        INSERT INTO plans (name, description, price, duration_days, traffic_gb, is_active, sort_order)
        VALUES
            ('Trial',    'Пробный период',               0,   7,  5,   1, 0),
            ('Базовый',  '30 дней, 30 ГБ',             300,  30, 30,  1, 1),
            ('Стандарт', '30 дней, 100 ГБ',            500,  30, 100, 1, 2),
            ('Безлимит', '30 дней, без лимита',         700,  30, 0,   1, 3)
    """)
    op.execute("INSERT OR IGNORE INTO server_settings (id) VALUES (1)")

def downgrade():
    op.execute("DELETE FROM plans")
    op.execute("DELETE FROM server_settings")
```

---

## 2.4 Настройки SQLite для production

```python
# app/database.py

from sqlalchemy import create_engine, event
from sqlalchemy.orm import sessionmaker, DeclarativeBase
import os

DATABASE_URL = os.getenv("DATABASE_URL", "sqlite:////opt/olcpanel/data/olcpanel.db")

engine = create_engine(
    DATABASE_URL,
    connect_args={
        "check_same_thread": False,  # нужно для FastAPI async
        "timeout": 30,               # ждать 30 сек если БД занята
    },
    echo=False,  # True для отладки SQL-запросов
    pool_pre_ping=True,
)

# Включаем критически важные PRAGMA при каждом соединении
@event.listens_for(engine, "connect")
def set_sqlite_pragma(dbapi_connection, connection_record):
    cursor = dbapi_connection.cursor()
    cursor.execute("PRAGMA foreign_keys = ON")     # включить FK constraints
    cursor.execute("PRAGMA journal_mode = WAL")    # Write-Ahead Logging: лучше concurrent reads
    cursor.execute("PRAGMA synchronous = NORMAL")  # баланс скорость/надёжность
    cursor.execute("PRAGMA cache_size = -64000")   # 64 МБ кэш страниц
    cursor.execute("PRAGMA temp_store = MEMORY")   # временные таблицы в RAM
    cursor.execute("PRAGMA mmap_size = 268435456") # memory-mapped I/O, 256 МБ
    cursor.close()

SessionLocal = sessionmaker(autocommit=False, autoflush=False, bind=engine)

class Base(DeclarativeBase):
    pass

# Dependency для FastAPI
def get_db():
    db = SessionLocal()
    try:
        yield db
    finally:
        db.close()
```

**Почему эти PRAGMA важны:**

| PRAGMA | Значение | Зачем |
|--------|----------|-------|
| `foreign_keys = ON` | Включает проверку FK | По умолчанию ВЫКЛЮЧЕНО в SQLite — это опасно |
| `journal_mode = WAL` | Write-Ahead Logging | Позволяет читать БД пока идёт запись — нужно для FastAPI |
| `synchronous = NORMAL` | Не fsync каждую транзакцию | В 2-3 раза быстрее чем FULL, при потере питания рискуем потерять только последнюю транзакцию |
| `cache_size = -64000` | 64 МБ страничного кэша | Ускоряет повторные запросы |
| `mmap_size` | 256 МБ mmap | Быстрый доступ к файлу через mmap (Linux) |

---

## 2.5 Полная SQL-схема (одним блоком для Alembic)

```sql
-- ============================================================
-- OlcPanel Database Schema v2.0
-- SQLite 3, WAL mode, FK enabled
-- Канон: clients + user_devices(device_id) + servers + authz_state.
-- LEGACY: configs (история). DORMANT: traffic_daily/traffic_snapshots.
-- ============================================================

PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;

-- 1. Administrators
CREATE TABLE IF NOT EXISTS admins (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    username        TEXT    NOT NULL UNIQUE,
    password_hash   TEXT    NOT NULL,
    created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
    last_login      DATETIME NULL,
    is_active       BOOLEAN NOT NULL DEFAULT 1
);

-- 2. Plans
CREATE TABLE IF NOT EXISTS plans (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT    NOT NULL UNIQUE,
    description   TEXT    NULL,
    price         INTEGER NOT NULL DEFAULT 0,
    duration_days INTEGER NOT NULL DEFAULT 30,
    traffic_gb    INTEGER NOT NULL DEFAULT 0,
    is_active     BOOLEAN NOT NULL DEFAULT 1,
    sort_order    INTEGER NOT NULL DEFAULT 0,
    created_at    DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at    DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- 3. Clients
CREATE TABLE IF NOT EXISTS clients (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL,
    telegram    TEXT    NULL,
    phone       TEXT    NULL,
    notes       TEXT    NULL,
    status      TEXT    NOT NULL DEFAULT 'active'
                CHECK(status IN ('active','suspended','expired','deleted')),
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    deleted_at  DATETIME NULL
);
CREATE INDEX IF NOT EXISTS idx_clients_status     ON clients(status);
CREATE INDEX IF NOT EXISTS idx_clients_telegram   ON clients(telegram);
CREATE INDEX IF NOT EXISTS idx_clients_created_at ON clients(created_at);

-- 4. Configs  -- 🟠 LEGACY: история конфигов; НЕ источник доступа (см. 2.2.4)
CREATE TABLE IF NOT EXISTS configs (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    client_id     INTEGER NOT NULL REFERENCES clients(id) ON DELETE RESTRICT,
    crypto_key    TEXT    NOT NULL,
    room_name     TEXT    NOT NULL,
    room_url      TEXT    NOT NULL,
    jitsi_server  TEXT    NOT NULL,
    transport     TEXT    NOT NULL DEFAULT 'datachannel',
    region_label  TEXT    NULL,
    uri           TEXT    NOT NULL,
    status        TEXT    NOT NULL DEFAULT 'active'
                  CHECK(status IN ('active','revoked')),
    created_at    DATETIME NOT NULL DEFAULT (datetime('now')),
    revoked_at    DATETIME NULL,
    revoke_reason TEXT    NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_configs_crypto_key ON configs(crypto_key);
CREATE INDEX IF NOT EXISTS idx_configs_client_id         ON configs(client_id);
CREATE INDEX IF NOT EXISTS idx_configs_status            ON configs(status);

-- 5. Subscriptions
CREATE TABLE IF NOT EXISTS subscriptions (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    client_id        INTEGER NOT NULL REFERENCES clients(id) ON DELETE RESTRICT,
    plan_id          INTEGER NOT NULL REFERENCES plans(id) ON DELETE RESTRICT,
    status           TEXT    NOT NULL DEFAULT 'active'
                     CHECK(status IN ('active','expired','suspended','cancelled')),
    started_at       DATETIME NOT NULL,
    expires_at       DATETIME NOT NULL,
    traffic_limit_mb INTEGER NOT NULL DEFAULT 0,
    traffic_used_mb  INTEGER NOT NULL DEFAULT 0,
    created_at       DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at       DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_subscriptions_client_id  ON subscriptions(client_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_status     ON subscriptions(status);
CREATE INDEX IF NOT EXISTS idx_subscriptions_expires_at ON subscriptions(expires_at);

-- 5b. Servers (реестр флота + наблюдаемость applied-authz; поля LKG)
CREATE TABLE IF NOT EXISTS servers (
    id                     INTEGER PRIMARY KEY AUTOINCREMENT,
    name                   TEXT    NOT NULL,
    host                   TEXT    NOT NULL,
    port                   INTEGER NOT NULL DEFAULT 443,
    api_base               TEXT    NULL,
    status                 TEXT    NOT NULL DEFAULT 'active'
                           CHECK(status IN ('active','maintenance','offline')),
    priority               INTEGER NOT NULL DEFAULT 100,
    fail_mode              TEXT    NOT NULL DEFAULT 'lkg'
                           CHECK(fail_mode IN ('lkg','open','closed')),
    expected_authz_version INTEGER NOT NULL DEFAULT 0,
    applied_authz_version  INTEGER NULL,
    lkg_valid_at           DATETIME NULL,
    load_errors            INTEGER NOT NULL DEFAULT 0,
    last_health_at         DATETIME NULL,
    notes                  TEXT    NULL,
    created_at             DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_servers_status ON servers(status);
-- дефолтный сервер для single-server (MVP); поля health заполняет self-poll
INSERT OR IGNORE INTO servers (id, name, host, port, status, priority)
    VALUES (1, 'default', 'localhost', 443, 'active', 100);

-- 5c. User Devices (deviceId/x-hwid — ЯДРО ДОСТУПА, мост к authz.json)
CREATE TABLE IF NOT EXISTS user_devices (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    client_id    INTEGER NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    device_id    TEXT    NOT NULL UNIQUE,
    label        TEXT    NULL,
    server_id    INTEGER NULL REFERENCES servers(id) ON DELETE SET NULL,
    is_active    BOOLEAN NOT NULL DEFAULT 1,
    last_used_at DATETIME NULL,
    created_at   DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_user_devices_client ON user_devices(client_id);
CREATE INDEX IF NOT EXISTS idx_user_devices_server ON user_devices(server_id);
CREATE INDEX IF NOT EXISTS idx_user_devices_active ON user_devices(is_active);

-- 5d. Authz State (монотонная версия allowlist; источник version в authz.json)
CREATE TABLE IF NOT EXISTS authz_state (
    id          INTEGER PRIMARY KEY DEFAULT 1 CHECK(id = 1),
    version     INTEGER NOT NULL DEFAULT 0,
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    allow_count INTEGER NOT NULL DEFAULT 0,
    last_writer TEXT    NULL
);
INSERT OR IGNORE INTO authz_state (id, version) VALUES (1, 0);

-- Добавить server_id в subscriptions (фундамент флота, nullable на Этапе 1):
--   ALTER TABLE subscriptions ADD COLUMN server_id INTEGER NULL REFERENCES servers(id);

-- 6. Payments
CREATE TABLE IF NOT EXISTS payments (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    client_id       INTEGER NOT NULL REFERENCES clients(id) ON DELETE RESTRICT,
    subscription_id INTEGER NULL REFERENCES subscriptions(id) ON DELETE SET NULL,
    amount          INTEGER NOT NULL DEFAULT 0,
    currency        TEXT    NOT NULL DEFAULT 'RUB',
    method          TEXT    NOT NULL DEFAULT 'card'
                    CHECK(method IN ('card','crypto','cash','other')),
    period_days     INTEGER NOT NULL DEFAULT 30,
    note            TEXT    NULL,
    created_at      DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_payments_client_id  ON payments(client_id);
CREATE INDEX IF NOT EXISTS idx_payments_created_at ON payments(created_at);

-- 7. Traffic Daily  -- 🟤 DORMANT: учёт не настроен (Решение 5); джоба не запускается
CREATE TABLE IF NOT EXISTS traffic_daily (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    client_id   INTEGER NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    date        DATE    NOT NULL,
    bytes_in    INTEGER NOT NULL DEFAULT 0,
    bytes_out   INTEGER NOT NULL DEFAULT 0,
    bytes_total INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_traffic_daily_client_date ON traffic_daily(client_id, date);
CREATE INDEX IF NOT EXISTS idx_traffic_daily_date ON traffic_daily(date);

-- 8. Traffic Snapshots  -- 🟤 DORMANT: iptables-аккаунтинга нет; не наполняется
CREATE TABLE IF NOT EXISTS traffic_snapshots (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    client_id       INTEGER NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    captured_at     DATETIME NOT NULL DEFAULT (datetime('now')),
    bytes_in_total  INTEGER NOT NULL DEFAULT 0,
    bytes_out_total INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_traffic_snapshots_client_captured ON traffic_snapshots(client_id, captured_at);
CREATE INDEX IF NOT EXISTS idx_traffic_snapshots_captured_at     ON traffic_snapshots(captured_at);

-- 9. Server Settings (singleton)
CREATE TABLE IF NOT EXISTS server_settings (
    id                   INTEGER PRIMARY KEY DEFAULT 1 CHECK(id = 1),
    jitsi_server         TEXT    NOT NULL DEFAULT 'meet1.arbitr.ru',
    default_room_prefix  TEXT    NOT NULL DEFAULT 'olcrtc',
    region_label         TEXT    NOT NULL DEFAULT 'EU/Amsterdam',
    transport            TEXT    NOT NULL DEFAULT 'datachannel',
    dns_server           TEXT    NOT NULL DEFAULT '8.8.8.8:53',
    socks_port           INTEGER NOT NULL DEFAULT 8808,
    server_yaml_path     TEXT    NOT NULL DEFAULT '/root/olcrtc/server.yaml',
    binary_path          TEXT    NOT NULL DEFAULT '/root/olcrtc/build/olcrtc-linux-amd64',
    service_name         TEXT    NOT NULL DEFAULT 'olcrtc-server',
    warn_days_before     INTEGER NOT NULL DEFAULT 3,
    authz_json_path      TEXT    NOT NULL DEFAULT '/opt/olcrtc-panel/data/authz.json',
    authz_mode           TEXT    NOT NULL DEFAULT 'off',
    authz_fail_mode      TEXT    NOT NULL DEFAULT 'lkg',
    lkg_max_age_sec      INTEGER NOT NULL DEFAULT 86400,
    updated_at           DATETIME NOT NULL DEFAULT (datetime('now'))
);
INSERT OR IGNORE INTO server_settings (id) VALUES (1);

-- 10. Audit Log
CREATE TABLE IF NOT EXISTS audit_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    admin_id    INTEGER NULL REFERENCES admins(id) ON DELETE SET NULL,
    action      TEXT    NOT NULL,
    entity_type TEXT    NULL,
    entity_id   INTEGER NULL,
    details     TEXT    NULL,
    ip_address  TEXT    NULL,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_audit_log_admin_id   ON audit_log(admin_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_created_at ON audit_log(created_at);
CREATE INDEX IF NOT EXISTS idx_audit_log_entity     ON audit_log(entity_type, entity_id);

-- Seed data: plans
INSERT OR IGNORE INTO plans (id, name, description, price, duration_days, traffic_gb, is_active, sort_order) VALUES
    (1, 'Trial',    'Пробный период. Бесплатно, 7 дней, 5 ГБ',         0,   7,  5,  1, 0),
    (2, 'Базовый',  'Для лёгкого использования. 30 дней, 30 ГБ',      300,  30, 30, 1, 1),
    (3, 'Стандарт', 'Оптимальный выбор. 30 дней, 100 ГБ',             500,  30, 100,1, 2),
    (4, 'Безлимит', 'Для активных пользователей. 30 дней, без лимита', 700, 30, 0,  1, 3);
```

---

*Следующий раздел: [[OlcPanel_03_Архитектура_сервисов]] — компоненты `Gate`(LKG)/`authz_writer`/`Coordinator`/`Pusher`/`HealthService`, модули приложения, структура директорий, взаимодействие.*

════════════════════════════════════════════════════════════════════════════════
<!-- Конец файла 02_Модель_данных.md -->
════════════════════════════════════════════════════════════════════════════════
