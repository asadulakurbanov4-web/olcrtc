════════════════════════════════════════════════════════════════════════════════
<!-- ФАЙЛ 4/10: 03_Архитектура_сервисов.md -->
════════════════════════════════════════════════════════════════════════════════

# OlcPanel — Раздел 3: Архитектура сервисов
## Компонентная диаграмма · Модули · Структура директорий · Взаимодействие · Границы

> **Версия:** 2.0 | **Статус:** 🟡 В разработке
> **Зависимости:** [[OlcPanel_00_Введение]] (Решения 1–5), [[OlcPanel_01_Обзор_и_требования]], [[OlcPanel_02_Модель_данных]], разделы 4 и 5.2 в [[OlcPanel_Анализ_Marzban_3xui]]
>
> ⚠️ **Что изменилось в v2.0 сервисного слоя.** Ядром стал **сервис доступа** — `authz_writer` (пишет `authz.json` атомарно + монотонный `version`) и серверный `Gate` (`internal/authz/authz.go`) в режиме **last-known-good** (Решение 4). Сервис `config_service` (пер-клиентский `crypto_key`) переориентирован в выдачу ссылки подписки на **общую** комнату/ключ + привязку устройства; `traffic_service` помечен **DORMANT** (Решение 5); `scheduler` блокирует через `authz.json`, а не через iptables. Добавлены сервисы координации флота (`ServerRegistry`, `MultiServerAuthzCoordinator`, `ServerAuthzPusher`, `ServerHealthService`) — детально в секции 3.4.x.

---

## 3.1 Принцип организации: монолит с чёткими модульными границами

### 3.1.1 Почему монолит, а не микросервисы

OlcPanel — это инструмент одного администратора для 10–20 клиентов. На VPS с 1.9 GB RAM
микросервисная архитектура создала бы:

- Дополнительные 3–5 процессов → +300–500 MB RAM
- Сетевой оверхед между сервисами (даже на localhost)
- Сложность деплоя и мониторинга
- Не нужную для данного масштаба отказоустойчивость

**Выбор:** Единый FastAPI-монолит с жёсткими модульными границами внутри. Модули
взаимодействуют через прямые вызовы Python-функций, а не HTTP. Это позволяет:
- Легко разделить на сервисы в будущем (при росте до 200+ клиентов)
- Держать весь код в одном репозитории
- Деплоить одним systemd-юнитом
- Дебажить без распределённых трейсов

### 3.1.3 Эволюция для горизонтального масштабирования (горизонтальный scaling + central panel)

При переходе на несколько olcrtc-серверов (дешёвые VPS, 50–100 клиентов на сервер, gold carriers + failover) монолитная панель эволюционирует, но сохраняет монолитный характер на ранних этапах (1–4 сервера).

**Новые/расширяемые модули внутри монолита (или лёгкие сервисы при росте):**
- **ServerRegistry / ServersService** — CRUD серверов (host, api_base, priority, status, version olcrtc). Health-check (ping + /health/carriers). Выбор сервера при выдаче подписки (по приоритету, нагрузке, гео).
- **MultiServerAuthzCoordinator** — внутренний компонент центральной панели (см. подробное описание ответственности, границ, алгоритма и failure modes в olcrtc-panel-architecture.md). Отвечает за вычисление актуального allow/deny для конкретного server_id и вызов механизма доставки. Не выбирает сервер и не является единой точкой принятия решения об авторизации — он только обеспечивает доставку актуального состояния из БД панели на целевые olcrtc-серверы.
- **SubscriptionOrchestrator** — генерация bundle с учётом `server_id` (в ссылке/бандле адрес нужного olcrtc или туннель). Переиспользует текущий `services/olcrtc.py` (parser server.yaml профилей/gold carriers) + расширяет на multi.
- **ClientBindingService** — привязка `user_devices` к конкретному `servers.id` (см. обновлённую модель в OlcPanel_02). Поддержка миграции клиента между серверами (смена server_id + перегенерация authz + уведомление).

**Граница при horizontal scaling (центральная панель):**
```
Центральная OlcPanel (монолит или с лёгким API layer)
├── ServerRegistry (список VPS, health, выбор при issue)
├── AuthzCoordinator (write_authz per server, синхронизация с Phase 1 Gate)
├── SubscriptionOrchestrator (bundle с server endpoint + carriers)
└── Routers / Services (как раньше, + multi-server endpoints)

          ↕ (HTTP / future gRPC / rsync authz.json)
OlcRTC Server 1 (VPS1, /opt/olcrtc, свой authz.json, свой server.yaml)
OlcRTC Server 2 (VPS2, ...)
...
```

**На старте (1–3 сервера):** можно держать одну панель на основном VPS и ручное/скриптовое управление остальными + одна общая БД. При 4+ — обязательный переход на central с health/приоритетами (см. roadmap в olcrtc-panel-architecture.md).

Это напрямую использует уже работающий Phase 1 (authz_writer + Gate + authz.json) и не требует немедленного усложнения (монолит легко разделить позже).

### 3.1.2 Граница монолита

```
┌─────────────────────────────────────────────────────────────────────┐
│                        OlcPanel Monolith                            │
│                   (один процесс, один порт 8000)                    │
│                                                                     │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────────┐   │
│  │  Routers │  │ Services │  │  Models  │  │   Schedulers     │   │
│  │  (HTTP)  │→ │(business │→ │(SQLAlch.)│  │  (APScheduler)   │   │
│  │          │  │  logic)  │  │          │  │                  │   │
│  └──────────┘  └──────────┘  └──────────┘  └──────────────────┘   │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
         ↕ HTTP                              ↕ subprocess/systemd
┌────────────────┐                  ┌────────────────────────────────┐
│  Nginx (proxy) │                  │  OlcRTC Server (отдельный      │
│  + static      │                  │  процесс, systemd-сервис)      │
│  + HTTPS       │                  │  /root/olcrtc/build/...        │
└────────────────┘                  └────────────────────────────────┘
         ↕ файловая система
┌────────────────────────────────────────────────────────────────────┐
│  SQLite DB  │  server.yaml  │  olcrtc logs  │  backups            │
│  (WAL mode) │  (read-only)  │  (journald)   │  (daily cron)       │
└────────────────────────────────────────────────────────────────────┘
```

---

## 3.2 Полная компонентная диаграмма

```
                              INTERNET
                                 │
                         ┌───────▼────────┐
                         │     Nginx      │
                         │  :443 (HTTPS)  │
                         │  Let's Encrypt │
                         │  rate limit    │
                         │  security hdrs │
                         └───────┬────────┘
                                 │
              ┌──────────────────┼──────────────────┐
              │                  │                  │
      /static/*          /api/*              /health
   (HTML/CSS/JS)     (FastAPI app)         (plain txt)
              │                  │
              │         ┌────────▼─────────────────────────────────┐
              │         │            FastAPI Application            │
              │         │              (Uvicorn, port 8000)         │
              │         │                                           │
              │         │  ┌──────────────────────────────────┐    │
              │         │  │         API Routers               │    │
              │         │  │  /auth  /clients  /configs        │    │
              │         │  │  /subscriptions  /dashboard       │    │
              │         │  │  /settings  /health/carriers      │    │
              │         │  └────────────────┬─────────────────┘    │
              │         │                   │                       │
              │         │  ┌────────────────▼─────────────────┐    │
              │         │  │          Services Layer           │    │
              │         │  │                                   │    │
              │         │  │  AuthService      ConfigService   │    │
              │         │  │  ClientService    TrafficService  │    │
              │         │  │  SubService       ServerService   │    │
              │         │  │  SchedulerService AuditService    │    │
              │         │  └────────────────┬─────────────────┘    │
              │         │                   │                       │
              │         │  ┌────────────────▼─────────────────┐    │
              │         │  │         Repository Layer          │    │
              │         │  │  (SQLAlchemy Sessions + Queries)  │    │
              │         │  └────────────────┬─────────────────┘    │
              │         │                   │                       │
              │         │  ┌────────────────▼─────────────────┐    │
              │         │  │       APScheduler (embedded)      │    │
              │         │  │  check_expired  collect_traffic   │    │
              │         │  │  cleanup_logs   health_refresh    │    │
              │         │  └──────────────────────────────────┘    │
              │         └──────────────────────────────────────────┘
              │                   │              │
              │         ┌─────────▼──┐    ┌──────▼──────────────┐
              │         │  SQLite DB │    │  OlcRTC Server      │
              │         │  WAL mode  │    │  (systemd service)  │
              │         │  FK ON     │    │  /root/olcrtc/build │
              │         └────────────┘    │  server.yaml        │
              │                           │  journald logs      │
              │                           └─────────────────────┘
              │
    ┌─────────▼──────────────────────────────┐
    │          Static Frontend               │
    │  /opt/olcpanel/static/                 │
    │  index.html  login.html                │
    │  app.js  style.css  charts.js          │
    └────────────────────────────────────────┘
```

---

## 3.3 Структура директорий проекта

### 3.3.1 Полное дерево файлов

```
/opt/olcpanel/                          ← корень проекта (production)
│
├── .env                                ← секреты и конфигурация (chmod 600)
├── .env.example                        ← шаблон без секретов (в git)
├── requirements.txt                    ← Python-зависимости с версиями
├── alembic.ini                         ← конфигурация Alembic
│
├── app/                                ← основной Python-пакет
│   ├── __init__.py
│   ├── main.py                         ← точка входа FastAPI, lifespan, middleware
│   ├── config.py                       ← Settings (pydantic-settings, читает .env)
│   ├── database.py                     ← SQLAlchemy engine, session, PRAGMA setup
│   │
│   ├── models/                         ← SQLAlchemy ORM-модели (таблицы)
│   │   ├── __init__.py                 ← экспорт всех моделей
│   │   ├── admin.py                    ← Admin
│   │   ├── plan.py                     ← Plan
│   │   ├── client.py                   ← Client
│   │   ├── config.py                   ← Config
│   │   ├── subscription.py             ← Subscription
│   │   ├── payment.py                  ← Payment
│   │   ├── traffic.py                  ← TrafficDaily, TrafficSnapshot
│   │   ├── server_settings.py          ← ServerSettings
│   │   └── audit_log.py               ← AuditLog
│   │
│   ├── schemas/                        ← Pydantic v2 схемы (запросы/ответы API)
│   │   ├── __init__.py
│   │   ├── auth.py                     ← LoginRequest, TokenResponse
│   │   ├── client.py                   ← ClientCreate, ClientUpdate, ClientResponse
│   │   ├── config.py                   ← ConfigResponse, ConfigYaml, ConfigURI
│   │   ├── subscription.py             ← SubscriptionCreate, SubscriptionExtend
│   │   ├── payment.py                  ← PaymentCreate, PaymentResponse
│   │   ├── traffic.py                  ← TrafficDailyResponse, TrafficStats
│   │   ├── dashboard.py               ← DashboardStats, AlertItem
│   │   └── settings.py                ← ServerSettingsUpdate, CarrierHealth
│   │
│   ├── routers/                        ← FastAPI APIRouter (HTTP endpoints)
│   │   ├── __init__.py
│   │   ├── auth.py                     ← POST /api/auth/login, POST /api/auth/logout
│   │   ├── clients.py                  ← CRUD /api/clients
│   │   ├── configs.py                  ← /api/clients/{id}/config
│   │   ├── subscriptions.py            ← /api/clients/{id}/subscription
│   │   ├── payments.py                 ← /api/clients/{id}/payments
│   │   ├── traffic.py                  ← /api/clients/{id}/traffic
│   │   ├── dashboard.py               ← GET /api/dashboard
│   │   ├── settings.py                ← /api/settings
│   │   └── health.py                  ← /api/health, /api/health/carriers
│   │
│   ├── services/                       ← Бизнес-логика (не знает об HTTP)
│   │   ├── __init__.py
│   │   ├── auth_service.py             ← login, verify_token, revoke_token
│   │   ├── client_service.py           ← create_client, suspend, restore
│   │   ├── config_service.py           ← generate_config, revoke_config, to_yaml, to_uri
│   │   ├── subscription_service.py     ← create_subscription, extend, expire_check
│   │   ├── traffic_service.py          ← collect_snapshot, aggregate_daily, get_stats
│   │   ├── server_service.py           ← get_olcrtc_status, restart_olcrtc, get_logs
│   │   ├── carriers_service.py         ← parse_good_carriers, get_health, refresh_health
│   │   ├── audit_service.py            ← log_action (все изменения)
│   │   └── scheduler_service.py        ← setup_scheduler, все фоновые задачи
│   │
│   ├── repositories/                   ← Слой доступа к данным (SQLAlchemy queries)
│   │   ├── __init__.py
│   │   ├── admin_repo.py
│   │   ├── client_repo.py
│   │   ├── config_repo.py
│   │   ├── subscription_repo.py
│   │   ├── payment_repo.py
│   │   ├── traffic_repo.py
│   │   └── audit_repo.py
│   │
│   ├── middleware/                     ← FastAPI middleware
│   │   ├── __init__.py
│   │   ├── auth_middleware.py          ← JWT-валидация, get_current_admin dependency
│   │   ├── rate_limit.py              ← простой rate limiter (in-memory, per-IP)
│   │   ├── audit_middleware.py         ← логирование всех запросов
│   │   └── security_headers.py        ← X-Frame-Options, CSP, HSTS и др.
│   │
│   └── core/                          ← Утилиты и общие компоненты
│       ├── __init__.py
│       ├── security.py                 ← bcrypt hash/verify, JWT create/decode
│       ├── crypto.py                   ← secrets.token_hex, UUID generation
│       ├── exceptions.py              ← кастомные HTTPException наследники
│       └── logging.py                 ← настройка structlog / logging
│
├── alembic/                           ← Миграции базы данных
│   ├── env.py
│   ├── script.py.mako
│   └── versions/
│       ├── 001_initial_schema.py
│       ├── 002_seed_plans.py
│       └── 003_seed_server_settings.py
│
├── static/                            ← Frontend (отдаётся Nginx, не FastAPI)
│   ├── index.html                     ← SPA: дашборд (после логина)
│   ├── login.html                     ← Страница входа
│   ├── css/
│   │   ├── style.css                  ← основные стили
│   │   └── login.css                  ← стили страницы входа
│   └── js/
│       ├── app.js                     ← основная логика SPA (dashboard)
│       ├── login.js                   ← логика формы входа
│       ├── api.js                     ← HTTP-клиент (fetch wrapper)
│       ├── charts.js                  ← Chart.js инициализация и данные
│       └── utils.js                   ← форматирование дат, байт, QR-код
│
├── data/                              ← Runtime-данные (не в git)
│   └── olcpanel.db                    ← SQLite database file
│
├── logs/                              ← Логи приложения (не в git)
│   ├── app.log                        ← основной лог
│   └── audit.log                      ← аудит-лог (только append)
│
├── backups/                           ← Бэкапы БД (создаются cron)
│   └── olcpanel-2026-06-12.db.gz      ← пример
│
├── scripts/                           ← Вспомогательные скрипты
│   ├── install.sh                     ← установка с нуля (venv, nginx, systemd)
│   ├── backup.sh                      ← резервное копирование SQLite
│   ├── create_admin.py                ← создание первого администратора
│   └── check_health.sh                ← проверка статуса всех компонентов
│
└── systemd/                           ← systemd unit-файлы (копируются при install)
    ├── olcpanel.service               ← OlcPanel FastAPI
    └── olcpanel-backup.timer          ← таймер бэкапа
```

### 3.3.2 Что НЕ входит в репозиторий (`.gitignore`)

```gitignore
# Секреты
.env

# Данные
data/
logs/
backups/

# Python
__pycache__/
*.pyc
*.pyo
.venv/
venv/

# IDE
.idea/
.vscode/
*.swp

# OS
.DS_Store
Thumbs.db
```

---

## 3.4 Описание каждого модуля

### 3.4.1 `app/main.py` — Точка входа

```python
# Ответственность:
# - Создание FastAPI-приложения
# - Подключение всех роутеров
# - Настройка middleware (порядок важен)
# - lifespan: инициализация БД + запуск планировщика при старте,
#             остановка планировщика при завершении
# - Обработчики глобальных ошибок

from contextlib import asynccontextmanager
from fastapi import FastAPI
from fastapi.staticfiles import StaticFiles
from fastapi.middleware.cors import CORSMiddleware

from app.database import create_tables
from app.services.scheduler_service import start_scheduler, stop_scheduler
from app.middleware.security_headers import SecurityHeadersMiddleware
from app.middleware.rate_limit import RateLimitMiddleware
from app.routers import auth, clients, configs, subscriptions, payments
from app.routers import traffic, dashboard, settings, health
from app.config import get_settings

@asynccontextmanager
async def lifespan(app: FastAPI):
    # === STARTUP ===
    settings = get_settings()
    create_tables()                          # CREATE TABLE IF NOT EXISTS
    start_scheduler()                        # APScheduler
    yield
    # === SHUTDOWN ===
    stop_scheduler()

app = FastAPI(
    title="OlcPanel",
    version="1.0.0",
    docs_url="/api/docs",                    # Swagger только по /api/docs
    redoc_url=None,
    openapi_url="/api/openapi.json",
    lifespan=lifespan,
)

# Middleware (порядок: первый добавленный = последний выполненный)
app.add_middleware(SecurityHeadersMiddleware)  # security headers на все ответы
app.add_middleware(RateLimitMiddleware)        # rate limiting
app.add_middleware(
    CORSMiddleware,
    allow_origins=[get_settings().PANEL_URL],  # только наш домен
    allow_credentials=True,
    allow_methods=["GET", "POST", "PATCH", "DELETE"],
    allow_headers=["Content-Type", "Authorization"],
)

# API роутеры
app.include_router(auth.router,          prefix="/api/auth",          tags=["auth"])
app.include_router(clients.router,       prefix="/api/clients",       tags=["clients"])
app.include_router(configs.router,       prefix="/api/clients",       tags=["configs"])
app.include_router(subscriptions.router, prefix="/api/clients",       tags=["subscriptions"])
app.include_router(payments.router,      prefix="/api/clients",       tags=["payments"])
app.include_router(traffic.router,       prefix="/api/clients",       tags=["traffic"])
app.include_router(dashboard.router,     prefix="/api/dashboard",     tags=["dashboard"])
app.include_router(settings.router,      prefix="/api/settings",      tags=["settings"])
app.include_router(health.router,        prefix="/api/health",        tags=["health"])

# Статика (frontend) — только если Nginx не обслуживает
# В production Nginx отдаёт статику напрямую, FastAPI только /api/*
# app.mount("/", StaticFiles(directory="static", html=True), name="static")
```

---

### 3.4.2 `app/config.py` — Конфигурация

**Принцип:** Все настройки — через переменные окружения (.env файл). Никаких хардкодов в коде.

```python
# Все настройки читаются из .env через pydantic-settings
# Используется паттерн singleton через lru_cache

from pydantic_settings import BaseSettings
from functools import lru_cache

class Settings(BaseSettings):
    # === Обязательные (нет значения по умолчанию — падаем при старте) ===
    SECRET_KEY: str                          # минимум 32 символа, urandom
    ADMIN_USERNAME: str                      # логин первого администратора
    ADMIN_PASSWORD: str                      # пароль (будет захеширован)

    # === База данных ===
    DATABASE_URL: str = "sqlite:////opt/olcpanel/data/olcpanel.db"

    # === JWT ===
    JWT_ALGORITHM: str = "HS256"
    JWT_ACCESS_TOKEN_EXPIRE_MINUTES: int = 60 * 24  # 24 часа

    # === Панель ===
    PANEL_URL: str = "https://your-panel-domain.com"
    PANEL_HOST: str = "127.0.0.1"
    PANEL_PORT: int = 8000
    DEBUG: bool = False

    # === OlcRTC интеграция ===
    OLCRTC_SERVICE_NAME: str = "olcrtc-server"
    OLCRTC_BINARY_PATH: str = "/root/olcrtc/build/olcrtc-linux-amd64"
    OLCRTC_SERVER_YAML: str = "/root/olcrtc/server.yaml"
    GOOD_CARRIERS_MD_PATH: str = "/root/olcrtc/docs/good-carriers.md"

    # === Rate limiting ===
    LOGIN_RATE_LIMIT_ATTEMPTS: int = 5       # попыток
    LOGIN_RATE_LIMIT_WINDOW_SEC: int = 900   # за 15 минут

    # === Планировщик ===
    SCHEDULER_INTERVAL_TRAFFIC_SEC: int = 300   # сбор трафика каждые 5 мин
    SCHEDULER_INTERVAL_EXPIRE_SEC: int = 3600   # проверка истечений каждый час
    SCHEDULER_INTERVAL_HEALTH_SEC: int = 300    # обновление health carriers
    SCHEDULER_CLEANUP_HOUR: int = 4             # очистка логов в 04:00 UTC

    # === Безопасность ===
    BCRYPT_ROUNDS: int = 12
    WARN_DAYS_BEFORE_EXPIRE: int = 3

    class Config:
        env_file = ".env"
        env_file_encoding = "utf-8"

@lru_cache()
def get_settings() -> Settings:
    return Settings()
```

**Файл `.env.example`:**

```bash
# === ОБЯЗАТЕЛЬНО ЗАМЕНИТЬ ===
SECRET_KEY=GENERATE_WITH__python3_-c_"import secrets; print(secrets.token_hex(32))"
ADMIN_USERNAME=admin
ADMIN_PASSWORD=GENERATE_STRONG_PASSWORD_HERE

# === База данных ===
DATABASE_URL=sqlite:////opt/olcpanel/data/olcpanel.db

# === Панель ===
PANEL_URL=https://your-panel-domain.com

# === OlcRTC ===
OLCRTC_SERVICE_NAME=olcrtc-server
OLCRTC_BINARY_PATH=/root/olcrtc/build/olcrtc-linux-amd64
OLCRTC_SERVER_YAML=/root/olcrtc/server.yaml
GOOD_CARRIERS_MD_PATH=/root/olcrtc/docs/good-carriers.md
```

---

### 3.4.3 Services Layer — описание каждого сервиса

#### `auth_service.py`

```
Ответственность:
  - Проверка username/password против БД (bcrypt verify)
  - Создание JWT access token
  - In-memory revocation list (set() токенов; при logout/revoke добавляем jti)
  - Верификация токена (decode + проверка в revocation list)
  - Обновление last_login в БД
  - Логирование попыток входа через AuditService

Зависимости:
  - AdminRepository (чтение из admins)
  - AuditService (логирование)
  - app/core/security.py (bcrypt, JWT)

Revocation list:
  - Хранится в памяти (set из jti-строк)
  - При перезапуске сбрасывается — нормально, т.к. JWT expire 24ч
  - При росте до 200+ клиентов — переехать на Redis

Интерфейс:
  login(username, password, ip) -> TokenResponse | raise 401
  verify_token(token) -> Admin | raise 401
  logout(jti) -> None
  revoke_all_sessions(admin_id) -> None
```

#### `subscription_link_service.py` (бывший `config_service.py` — переориентирован в v2.0)

```
⚠️ v2.0: НЕ генерирует пер-клиентский crypto.key/комнату (Решение 1).
Ключ и комната ОБЩИЕ — берутся из server.yaml профилей (services/olcrtc.py).
Идентичность клиента — deviceId (user_devices), не ключ.

Ответственность:
  - Формирование ссылки подписки olcrtc:// на ОБЩУЮ комнату+ключ выбранного
    server_id (get_primary_profile / generate_uri из services/olcrtc.py)
  - Генерация subs-бандла (sub-токен olcrtc_key ведёт на /api/sub/{token})
  - Генерация QR ссылки подписки (base64 PNG)
  - Ротация sub-токена (старая ссылка → 404), БЕЗ смены общего ключа
  - НЕ трогает allow[] напрямую — привязка устройства идёт через bind_device
    в момент загрузки подписки (см. authz_service)

URI Format (общая комната+ключ из server.yaml):
  olcrtc://jitsi?{transport}@{room_url}#{shared_key}${region_label}
  (room_url и shared_key — серверные, одинаковые для всех клиентов сервера)

Subs Format (sub-токен на стороне панели):
  #name: olcpanel-{client_name}
  #update: {panel_url}/api/sub/{olcrtc_key}      ← персональный sub-токен
  #expires: {expires_at_iso}
  olcrtc://jitsi?datachannel@{room_url}#{shared_key}
  ##comment: expires: {expires_at} | re-validate recommended

ВАЖНО: при загрузке этой ссылки olcbox шлёт x-hwid → панель ловит его
в /api/sub/{token} → bind_device → authz_writer (см. authz_service).

Зависимости:
  - services/olcrtc.py (профили server.yaml, generate_uri, generate_subscription_bundle)
  - ClientRepository / DeviceRepository (sub-токен, server_id)
  - qrcode
```

> Старый `Config.crypto_key`/`carriers_snapshot` остаются как **LEGACY** ([[OlcPanel_02_Модель_данных]], 2.2.4) — только для истории, не для выдачи.

#### `subscription_service.py`

```
Ответственность:
  - Создание подписки при создании клиента
  - Продление (новая запись, expires_at = MAX(now, old_expires) + days)
  - Автопроверка истечения (вызывается APScheduler каждый час)
  - Автоотключение истёкших → user_devices.is_active=false + authz_writer (soft-block)
  - Возврат устройств в allow при продлении заблокированного → authz_writer
  (traffic_used_mb НЕ используется — Решение 5)

Алгоритм продления (важен):
  old_expires = текущая expires_at
  base = MAX(old_expires, datetime.utcnow())  # если уже истекла — от сегодня
  new_expires = base + timedelta(days=extension_days)
  if был soft-block: user_devices.is_active=true → write_authz_file(db) → push

Зависимости:
  - SubscriptionRepository, DeviceRepository
  - ClientRepository (обновление client.status)
  - authz_service / authz_writer (любое изменение доступа)
  - Coordinator (на multi-server), AuditService
```

#### `authz_service.py` / `authz_writer.py` — материализация allowlist (Phase 1, ядро монетизации)

```
Ответственность:
  - bind_device(client_id, device_id): UPSERT user_devices(device_id) при
    загрузке подписки (ловится в /api/sub/{token} по заголовку x-hwid).
  - write_authz_file(db): ПЕРЕСБОРКА allow[] из активных user_devices
    (is_active && подписка active && expires_at>now) для нужного server_id,
    атомарная запись authz.json, инкремент версии.
  - Триггеры записи: create/extend/block/unblock/delete клиента, sub-fetch,
    scheduler (expiry), ручной re-push.

Алгоритм write_authz_file (КРИТИЧЕСКИЕ инварианты — из 4.5/5.1/5.2):
  1. tx: authz_state.version += 1  (монотонно; на multi-server — per server_id)
  2. allow = SELECT device_id FROM user_devices d
             JOIN clients c ... JOIN subscriptions s ...
             WHERE d.is_active AND s.status='active' AND s.expires_at>now
               [AND d.server_id = :server_id]   # на флоте
  3. payload = {version, updated_at(UTC), mode, allow, deny}
  4. АТОМАРНО: write tmp → fsync(tmp) → os.rename(tmp, authz.json)
     (никаких дописываний на месте; rename атомарен в пределах ФС)
  5. гарантировать сдвиг mtime (os.utime при необходимости / дедуп частых записей)
  6. authz_state.allow_count = len(allow); audit_log("write_authz", version)
  7. (флот) Coordinator.on_change(server_id) → доставка (см. ниже)

Почему так: Gate перечитывает файл по mtime и в проде работает по
last-known-good (Решение 4). Неатомарная запись → потеря allow (отвал платящих)
или «зафиксированный» неполный срез; немонотонная version → откат allowlist.
Поэтому атомарность + version ОБЯЗАТЕЛЬНЫ с Этапа 1.

Зависимости:
  - DeviceRepository, SubscriptionRepository, AuthzStateRepository
  - ServerSettings (authz_json_path, authz_mode)
  - Coordinator (на multi-server), AuditService
```

#### Серверный `Gate` (`internal/authz/authz.go`, Go) — решение о доступе с last-known-good

```
Это НЕ часть Python-панели — это компонент olcrtc-сервера, потребитель authz.json.
Описан здесь, потому что его контракт диктует требования к authz_writer.

Состояние (v2.0, добавлено для LKG):
  cached     {mtime, version, allow set, deny set}   # текущее применённое
  lkgAllow   set                                     # ПОСЛЕДНИЙ ВАЛИДНЫЙ allow
  lkgValidAt time                                    # когда он был загружен
  loadErrors int                                     # счётчик ошибок чтения/парсинга
  failMode   enum{lkg, open, closed}                 # прод = lkg
  lkgMaxAge  duration                                # порог устаревания (эскалация)

Allowed(deviceID) (под мьютексом, ленивая перезагрузка по mtime):
  st, err := os.Stat(path)
  if err != nil:                       # файл пропал
      escalateIfStale(); return decideByLKG(deviceID)   # НЕ fail-open
  if st.mtime == cached.mtime:
      return decideByCached(deviceID)
  data, perr := parseAndValidate(path) # JSON + схема + version > cached.version
  if perr != nil:                      # битый/неполный/откат версии
      loadErrors++; alert(); escalateIfStale()
      return decideByLKG(deviceID)     # держим последнее ХОРОШЕЕ
  cached = data; lkgAllow = data.allow; lkgValidAt = now   # обновляем LKG ТОЛЬКО при успехе
  return decideByCached(deviceID)

escalateIfStale(): если now - lkgValidAt > lkgMaxAge → critical-alert
  (+ опционально перейти в closed как «жёсткий потолок» поверх lkg).

Холодный старт (нет ни одного валидного состояния в памяти):
  файл отсутствует/битый при запуске → cold-start policy = closed + громкий alert
  («никогда не видели валидный allowlist» ≠ «временно потеряли свежий»).

Sweep живых сессий: по EnforceInterval Gate проходит активные peer-сессии и
рвёт те, чей deviceID больше не в allow (removePeerSession) — блок применяется
к УЖЕ подключённым, а не только на reconnect.

Инвариант (тест AZ-07): в failMode=lkg битый/пропавший файл НЕ расширяет
множество разрешённых deviceID сверх последнего валидного.
```

#### `fleet/` — координация флота (скелет с Этапа 1, наполнение Этап 2–3)

```
ServerRegistry (servers):
  CRUD серверов; статус active/maintenance/offline; api_base; priority.
  Отдаёт health-поля (applied_authz_version, lkg_valid_at, load_errors).

IssuancePolicy:
  ЕДИНСТВЕННЫЙ, кто выбирает server_id для клиента (priority/нагрузка/гео).
  Координатор сервер НЕ выбирает.

MultiServerAuthzCoordinator.on_change(client, server_id):
  пересчитывает срез allow ДЛЯ server_id из БД (никогда не «дельтит»),
  вызывает Pusher, логирует, при устойчивой ошибке — alert + ручной re-push.
  Best-effort: не дошло до одного — остальные работают.

ServerAuthzPusher (абстракция доставки, скрывает транспорт):
  - LocalFilePusher  — Этап 1: вызывает authz_writer для дефолтного сервера (no-op флота)
  - RsyncPusher (A)  — Этап 2: payload → rsync → trigger (атомарный rename на сервере)
  - HttpPusher  (B)  — Этап 3: POST /internal/authz/update (mTLS/подпись),
                       сервер отвергает version ≤ applied, 200 ТОЛЬКО после записи

ServerHealthService:
  poll /health каждого сервера → пишет в servers: applied_authz_version,
  lkg_valid_at, load_errors, last_health_at. Это детектор расхождений
  (split-brain, 5.3) и устаревания LKG. Расхождение applied<expected или
  load_errors>0 или устаревший lkg_valid_at → «Требуют внимания» + alert.
```

> Полные потоки доставки, миграция устройства (порядок «добавить на новый → убрать со старого») и split-brain — раздел 4 в [[OlcPanel_Анализ_Marzban_3xui]]; API доставки — [[OlcPanel_04_API]]; операционка — [[OlcPanel_07_Инфраструктура]]; угрозы — [[OlcPanel_08_Безопасность]].

#### `traffic_service.py` — 🟤 DORMANT (в v2.0 не активен)

```
⚠️ DORMANT (Решение 5): пер-клиентский трафик на WebRTC/Jitsi-туннеле не
измеряется (нет Xray-stats/iptables-MARK по клиенту). Сервис и таблицы
traffic_* оставлены «спящими» под будущую серверную телеметрию (Этап 3).
В UI трафик показывается как «лимит N GB · учёт не настроен».
Джобы сбора НЕ регистрируются в scheduler.

(Исторический iptables-подход не применяется: общая комната не даёт
пер-клиентской привязки счётчика.)
```

#### `carriers_service.py`

```
Ответственность:
  - Парсинг /root/olcrtc/docs/good-carriers.md
  - Чтение /tmp/validate-*.log (результаты валидации)
  - Формирование структуры CarrierHealth для /api/health/carriers
  - Кэширование (in-memory, TTL=5 минут)
  - Обновление по расписанию (APScheduler каждые 5 мин)

CarrierHealth структура:
  {
    "name": "ct.placetime.team",
    "status": "gold",              # gold / fallback / degraded / unknown
    "last_validated": "2026-06-12T14:00:00Z",
    "soak_kb": 104,                # сколько KB передано при последнем soak
    "volume_aware": true,
    "anonymous": true,
    "is_primary": true
  }

Парсинг good-carriers.md:
  Ищем строки с маркерами GOLD, fallback, ct., mf.
  Читаем комментарии с метриками (100KB+, volume_aware, ANONYMOUS).
  Не парсим весь MD — только структурированные секции.

Зависимости:
  - ServerSettings (GOOD_CARRIERS_MD_PATH)
  - файловая система (чтение MD и validate-логов)
```

#### `server_service.py`

```
Ответственность:
  - Проверка статуса OlcRTC-сервера (systemctl is-active)
  - Перезапуск сервера (systemctl restart) — с проверкой прав
  - Чтение последних N строк логов (journalctl -u olcrtc-server -n 100)
  - Обновление server.yaml при смене Jitsi-сервера
  - Перегенерация конфигов всех активных клиентов при смене Jitsi

Безопасность subprocess:
  - Whitelist разрешённых команд (enum)
  - Никаких shell=True
  - Timeout на каждый вызов (5 сек)
  - Только конкретные команды systemctl (is-active, restart, status)
  - Запуск от non-root пользователя с sudo только для systemctl
    (настраивается в /etc/sudoers.d/olcpanel)

Интерфейс:
  get_status() -> Literal["running", "stopped", "unknown"]
  restart() -> bool
  get_logs(lines=100) -> list[str]
  update_jitsi_server(new_server: str) -> None  # обновляет server.yaml + regen configs
```

#### `scheduler_service.py`

```
Ответственность:
  - Инициализация APScheduler при старте приложения
  - Регистрация всех периодических задач
  - Корректная остановка при shutdown

Задачи (v2.0):
  1. check_expired_subscriptions()  [ОСНОВНАЯ, монетизация]
     Каждый час. Находит active subscriptions с expires_at < now().
     status → expired; user_devices.is_active=false для устройств клиента.
     → authz_writer.write_authz_file(db)  (атомарно, version++)
     → (флот) Coordinator.on_change(server_id) → push.
     Пишет audit_log. БЛОКИРОВКА ЧЕРЕЗ authz.json, НЕ iptables.

  2. warn_expiring_subscriptions()
     Каждый час/раз в день. Подписки с expires_at в пределах warn_days_before →
     отметка «Требуют внимания» + (Этап 2) Telegram-напоминание.

  3. refresh_fleet_health()   [Этап 2; на single-server — self-poll]
     Каждые N минут. ServerHealthService.poll(): для каждого server_id
     обновляет applied_authz_version / lkg_valid_at / load_errors / last_health_at.
     Расхождение applied<expected или load_errors>0 или устаревший lkg_valid_at
     → «Требуют внимания» + alert (5.3).

  4. backup_database()
     Ежедневно в 03:00 UTC. Копирует SQLite + authz.json + server.yaml в backups/
     с датой; хранит 7 дней (5.2/5.4 — БД источник пересборки authz).

  ✗ collect_traffic_snapshots() / cleanup_old_snapshots() — НЕ регистрируются
    (traffic_service DORMANT, Решение 5).

APScheduler конфигурация:
  - BackgroundScheduler (не AsyncIO — проще интегрировать с blocking ops)
  - JobStore: MemoryJobStore (задачи регистрируются при каждом старте)
  - Executor: ThreadPoolExecutor(max_workers=2)
  - Каждая задача оборачивается в try/except с логированием ошибок

Важно: задачи не должны блокировать друг друга.
  Если collect_traffic занимает > 5 мин → misfire_grace_time=60.
  Задачи идемпотентны (повторный запуск не вызывает дублирование данных).
```

#### `audit_service.py`

```
Ответственность:
  - Запись всех мутирующих операций в audit_log
  - Единый интерфейс для всех сервисов (не дублировать)
  - Сериализация details в JSON

Вызывается из:
  - auth_service (login, logout, failed_login)
  - client_service (create, update, suspend, delete)
  - config_service (generate, revoke)
  - subscription_service (create, extend, expire, suspend)
  - payment_service (record_payment)
  - server_service (restart, update_settings)

Интерфейс:
  log(
      db: Session,
      action: str,           # "client.create", "config.revoke", ...
      admin_id: int | None,
      entity_type: str | None,
      entity_id: int | None,
      details: dict | None,
      ip_address: str | None,
  ) -> None

Формат action:
  {entity}.{operation}
  Примеры: "client.create", "client.suspend", "config.revoke",
           "subscription.extend", "auth.login", "auth.login_failed",
           "settings.update", "olcrtc.restart"
```

---

### 3.4.4 Repository Layer

Репозиторий — единственное место, где пишется SQL (через SQLAlchemy ORM). Сервисы **не должны** знать о Session, query, filter напрямую.

**Пример структуры репозитория:**

```python
# app/repositories/client_repo.py

from sqlalchemy.orm import Session
from app.models.client import Client, ClientStatus
from typing import Optional

class ClientRepository:

    def get_by_id(self, db: Session, client_id: int) -> Optional[Client]:
        return db.query(Client).filter(
            Client.id == client_id,
            Client.deleted_at.is_(None)
        ).first()

    def get_all(
        self,
        db: Session,
        status: Optional[ClientStatus] = None,
        search: Optional[str] = None,
        skip: int = 0,
        limit: int = 100,
    ) -> list[Client]:
        q = db.query(Client).filter(Client.deleted_at.is_(None))
        if status:
            q = q.filter(Client.status == status)
        if search:
            q = q.filter(
                Client.name.ilike(f"%{search}%") |
                Client.telegram.ilike(f"%{search}%") |
                Client.notes.ilike(f"%{search}%")
            )
        return q.order_by(Client.created_at.desc()).offset(skip).limit(limit).all()

    def create(self, db: Session, **kwargs) -> Client:
        client = Client(**kwargs)
        db.add(client)
        db.commit()
        db.refresh(client)
        return client

    def update(self, db: Session, client: Client, **kwargs) -> Client:
        for key, value in kwargs.items():
            setattr(client, key, value)
        db.commit()
        db.refresh(client)
        return client

    def soft_delete(self, db: Session, client: Client) -> Client:
        from datetime import datetime
        return self.update(db, client,
                           status=ClientStatus.DELETED,
                           deleted_at=datetime.utcnow())

    def count_by_status(self, db: Session) -> dict[str, int]:
        from sqlalchemy import func
        results = db.query(Client.status, func.count(Client.id))\
                    .filter(Client.deleted_at.is_(None))\
                    .group_by(Client.status)\
                    .all()
        return {status: count for status, count in results}
```

---

### 3.4.5 Middleware

#### `auth_middleware.py` — FastAPI Dependency

```python
# Используется как Depends() в роутерах, а не как Middleware class.
# Это стандартный FastAPI-подход для per-route авторизации.

from fastapi import Depends, HTTPException, Cookie
from app.services.auth_service import AuthService

async def get_current_admin(
    access_token: str | None = Cookie(default=None),
    auth_service: AuthService = Depends(),
) -> Admin:
    if not access_token:
        raise HTTPException(status_code=401, detail="Not authenticated")
    admin = auth_service.verify_token(access_token)
    if not admin:
        raise HTTPException(status_code=401, detail="Invalid or expired token")
    return admin

# Использование в роутере:
# @router.get("/clients")
# async def list_clients(admin = Depends(get_current_admin), ...):
#     ...
```

#### `rate_limit.py` — Simple In-Memory Rate Limiter

```
Реализация: sliding window per IP
Хранение: dict[ip -> deque[timestamp]]
Лимиты:
  - /api/auth/login: 5 попыток за 15 минут
  - /api/*: 100 запросов в минуту
  - /api/health: без лимита

При превышении: HTTP 429 Too Many Requests
  + Retry-After header
  + Запись в audit_log (action="rate_limit.exceeded")

Важно: in-memory dict очищается при перезагрузке.
При production-масштабе (много workers) → Redis.
Для одного воркера (наш случай) — достаточно.
```

#### `security_headers.py`

```python
# Все ответы FastAPI получают эти заголовки:

SECURITY_HEADERS = {
    "X-Frame-Options": "DENY",
    "X-Content-Type-Options": "nosniff",
    "X-XSS-Protection": "1; mode=block",
    "Referrer-Policy": "strict-origin-when-cross-origin",
    "Permissions-Policy": "camera=(), microphone=(), geolocation=()",
    # CSP настраивается в Nginx (см. 07_Инфраструктура)
}
```

---

## 3.5 Потоки данных (Data Flows)

### 3.5.1 Flow: Создание нового клиента

```
Admin Browser
    │
    │ POST /api/clients
    │ Body: {name, telegram, plan_id, notes}
    │ Cookie: access_token=<jwt>
    ↓
Nginx (proxy_pass → FastAPI :8000)
    ↓
RateLimitMiddleware (check: <100 req/min)
    ↓
SecurityHeadersMiddleware
    ↓
Router: clients.create_client()
    │
    ├─ Depends(get_current_admin) → AuthService.verify_token() → Admin
    ├─ Validate body (Pydantic ClientCreate schema)
    │
    ↓
ClientService.create_client(db, admin, data)
    │
    ├─ ClientRepository.create(db, name=..., telegram=..., status="active")
    │       → INSERT INTO clients ... → Client(id=5)
    │
    ├─ PlanRepository.get_by_id(db, plan_id=3)
    │       → Plan(id=3, duration_days=30, traffic_gb=100)
    │
    ├─ SubscriptionService.create_subscription(db, client_id=5, plan)
    │       → started_at = now()
    │       → expires_at = now() + 30 days
    │       → INSERT INTO subscriptions ... → Subscription(id=7)
    │
    ├─ ConfigService.generate_config(db, client_id=5)
    │       → crypto_key = secrets.token_hex(32)    # 64 hex chars
    │       → room_name = "olcrtc-" + uuid4()[:8]  # "olcrtc-a1b2c3d4"
    │       → settings = ServerSettingsRepository.get(db)
    │       → room_url = f"https://{settings.jitsi_server}/{room_name}"
    │       → uri = build_uri(crypto_key, room_url, transport, region)
    │       → carriers_snapshot = CarriersService.get_current_gold()  # JSON
    │       → INSERT INTO configs ... → Config(id=3)
    │
    ├─ AuditService.log(db, "client.create", admin_id=1, entity_type="client",
    │                   entity_id=5, details={name, plan_id}, ip=request.client.host)
    │       → INSERT INTO audit_log ...
    │
    └─ return ClientResponse(id=5, ..., config=ConfigResponse(...))

    ↓
Router возвращает JSON 201 Created
    ↓
Nginx добавляет security headers
    ↓
Admin Browser: показывает URI + QR-код
```

### 3.5.2 Flow: APScheduler — проверка истёкших подписок

```
APScheduler (каждый час, фоновый поток)
    │
    ↓
scheduler_service.check_expired_subscriptions()
    │
    ├─ db = SessionLocal()
    │
    ├─ SubscriptionRepository.get_expired_active(db)
    │       → SELECT * FROM subscriptions
    │         WHERE status='active' AND expires_at < datetime('now')
    │       → [Subscription(id=7, client_id=2, ...)]
    │
    ├─ FOR EACH expired subscription:
    │   │
    │   ├─ SubscriptionRepository.update(db, sub, status="expired")
    │   │
    │   ├─ ClientRepository.update(db, client, status="expired")
    │   │
    │   ├─ TrafficService.remove_iptables_rule(client_id=2)
    │   │       → subprocess(["iptables", "-D", "FORWARD", ...], timeout=5)
    │   │
    │   └─ AuditService.log(db, "subscription.auto_expired",
    │                        admin_id=None, entity_type="subscription",
    │                        entity_id=7, details={"reason": "expired_at passed"})
    │
    ├─ db.close()
    └─ logger.info(f"Expired {n} subscriptions")
```

### 3.5.3 Flow: Получение health carriers

```
Admin Browser
    │ GET /api/health/carriers
    ↓
health.router.get_carriers_health()
    │
    ├─ Depends(get_current_admin)  ← авторизация обязательна
    │
    ↓
CarriersService.get_health()
    │
    ├─ Проверяем in-memory cache (TTL 5 мин)
    │   ├─ Cache hit → return cached data
    │   └─ Cache miss → refresh()
    │
    CarriersService.refresh()
        │
        ├─ Читаем good-carriers.md (settings.GOOD_CARRIERS_MD_PATH)
        │   → парсим секции GOLD, fallback, метрики
        │
        ├─ Читаем /tmp/validate-*.log (если есть)
        │   → парсим last soak_kb, last_validate timestamp
        │
        ├─ Читаем journalctl -u olcrtc-server -n 200 --no-pager
        │   → grep liveness|stall|bytes|volume
        │   → определяем suspect carriers
        │
        └─ Собираем CarrierHealth[] → сохраняем в cache
    │
    └─ return CarrierHealth[] (JSON)

Response:
{
  "carriers": [
    {
      "name": "ct.placetime.team",
      "status": "gold",
      "last_validated": "2026-06-12T10:00:00Z",
      "soak_kb": 104,
      "volume_aware": true,
      "anonymous": true,
      "is_primary": true,
      "suspect": false
    },
    {
      "name": "mf.example.com",
      "status": "fallback",
      "last_validated": "2026-06-12T09:30:00Z",
      "soak_kb": 45,
      "volume_aware": false,
      "anonymous": false,
      "is_primary": false,
      "suspect": false
    }
  ],
  "updated_at": "2026-06-12T14:05:00Z"
}
```

### 3.5.4 Flow: Смена Jitsi-сервера (массовое обновление)

```
Admin: POST /api/settings {jitsi_server: "meet2.example.com"}
    ↓
SettingsRouter → ServerService.update_jitsi_server("meet2.example.com")
    │
    ├─ 1. Обновляем server_settings в БД
    │
    ├─ 2. Обновляем server.yaml на диске
    │       → читаем текущий server.yaml
    │       → меняем jitsi_server поле
    │       → записываем обратно (атомарно: temp file + rename)
    │
    ├─ 3. Перезапускаем OlcRTC-сервер
    │       → subprocess(["systemctl", "restart", "olcrtc-server"])
    │       → ждём 3 секунды
    │       → проверяем статус
    │
    ├─ 4. Перегенерируем конфиги всех активных клиентов
    │       → ConfigRepository.get_all_active(db)
    │       → FOR EACH:
    │           - revoke текущий config (status="revoked", revoke_reason="jitsi_server_changed")
    │           - generate новый config с новым jitsi_server
    │           - carriers_snapshot = {new_jitsi_server}
    │
    ├─ 5. Логируем в audit
    │       → "settings.jitsi_server_changed" + old/new values + n_configs_regenerated
    │
    └─ return {ok: true, regenerated_configs: 14, new_jitsi_server: "meet2.example.com"}

Admin: видит уведомление "Конфиги пересозданы. Разошлите клиентам новые URI."
```

---

## 3.6 Взаимодействие с OlcRTC-сервером

### 3.6.1 Что панель делает с OlcRTC

| Действие | Как | Права |
|----------|-----|-------|
| Проверить статус | `systemctl is-active olcrtc-server` | sudo (whitelist) |
| Перезапустить | `systemctl restart olcrtc-server` | sudo (whitelist) |
| Читать логи | `journalctl -u olcrtc-server -n 100` | journald group |
| Читать server.yaml | `open(path, 'r')` | chmod 644 |
| Обновить server.yaml | `open(path, 'w')` → temp+rename | chmod 644 |
| Читать good-carriers.md | `open(path, 'r')` | chmod 644 |
| Читать validate-*.log | `open('/tmp/validate-*.log')` | chmod 644 |

### 3.6.2 sudoers конфигурация

```sudoers
# /etc/sudoers.d/olcpanel
# Пользователь olcpanel может управлять только olcrtc-server service

olcpanel ALL=(ALL) NOPASSWD: /bin/systemctl is-active olcrtc-server
olcpanel ALL=(ALL) NOPASSWD: /bin/systemctl restart olcrtc-server
olcpanel ALL=(ALL) NOPASSWD: /bin/systemctl status olcrtc-server
olcpanel ALL=(ALL) NOPASSWD: /sbin/iptables -n -L FORWARD -v -x
olcpanel ALL=(ALL) NOPASSWD: /sbin/iptables -t mangle -A PREROUTING *
olcpanel ALL=(ALL) NOPASSWD: /sbin/iptables -t mangle -D PREROUTING *
```

### 3.6.3 Системный пользователь olcpanel

```bash
# Панель запускается от отдельного пользователя, не root
useradd --system --home /opt/olcpanel --shell /sbin/nologin olcpanel
chown -R olcpanel:olcpanel /opt/olcpanel

# Читать journald логи
usermod -a -G systemd-journal olcpanel
```

---

## 3.7 Dependency Graph (зависимости модулей)

```
main.py
├── config.py (Settings)
├── database.py (engine, session)
├── middleware/* (SecurityHeaders, RateLimit, CORS)
└── routers/*
    └── Depends(get_current_admin) → auth_middleware → AuthService
        └── AdminRepository → database.py

routers/clients.py → ClientService
    ├── ClientRepository → database.py
    ├── SubscriptionService
    │   └── SubscriptionRepository → database.py
    ├── ConfigService
    │   ├── ConfigRepository → database.py
    │   ├── ServerSettingsRepository → database.py
    │   └── CarriersService (для carriers_snapshot)
    │       └── файловая система (good-carriers.md, validate-*.log)
    └── AuditService → AuditRepository → database.py

scheduler_service.py
    ├── SubscriptionService (check_expired)
    ├── TrafficService (collect_snapshots)
    │   ├── TrafficRepository → database.py
    │   └── subprocess (iptables)
    ├── CarriersService (refresh_health)
    └── database.py (отдельная Session для каждой задачи)

routers/settings.py → ServerService
    ├── ServerSettingsRepository → database.py
    ├── ConfigService (regen all configs при смене Jitsi)
    ├── AuditService
    └── subprocess (systemctl, server.yaml r/w)
```

**Правило:** Нет циклических зависимостей. Направление всегда: Router → Service → Repository → Database.

---

## 3.8 Конфигурация зависимостей (requirements.txt)

```txt
# Web Framework
fastapi==0.111.0
uvicorn[standard]==0.29.0
python-multipart==0.0.9        # для form data

# Pydantic (настройки + схемы)
pydantic==2.7.1
pydantic-settings==2.3.0

# База данных
sqlalchemy==2.0.30
alembic==1.13.1

# Безопасность
passlib[bcrypt]==1.7.4         # bcrypt hashing
python-jose[cryptography]==3.3.0  # JWT

# QR-коды
qrcode[pil]==7.4.2
Pillow==10.3.0

# Планировщик
apscheduler==3.10.4

# Логирование
structlog==24.1.0

# Утилиты
python-dateutil==2.9.0
```

**Принципы:**
- Все версии зафиксированы (==, не >=) — воспроизводимые деплои
- Минимальный набор — нет лишних зависимостей
- После установки: `pip freeze > requirements.lock.txt` для точного snapshot

---

## 3.9 Производительность и ограничения монолита

### 3.9.1 Ожидаемые характеристики

| Метрика | Значение | Обоснование |
|---------|----------|-------------|
| RAM (FastAPI + Uvicorn) | ~80–120 МБ | Один воркер, SQLite в памяти |
| RAM (APScheduler) | ~5–10 МБ | Фоновые потоки |
| CPU в idle | <1% | Только планировщик раз в 5 мин |
| CPU при запросе (p95) | <50 мс | CRUD запросы, SQLite |
| CPU при генерации QR | <200 мс | Pillow + qrcode |
| Concurrent requests | ~50 | Один Uvicorn-воркер |
| SQLite write throughput | ~100 TPS | WAL mode, для 20 клиентов — избыток |

### 3.9.2 Узкие места и как с ними работать

**Проблема:** Uvicorn с одним воркером не параллелизует CPU-heavy задачи.

**Решение:** APScheduler использует ThreadPoolExecutor (не asyncio), поэтому тяжёлые задачи (iptables, subprocess) не блокируют event loop.

**Проблема:** SQLite не поддерживает параллельные writes (один writer at a time).

**Решение:** WAL mode позволяет читать одновременно с записью. Для 20 клиентов — не проблема никогда. При 200+ клиентах — миграция на PostgreSQL (одна строка в `.env`).

**Проблема:** Генерация QR-кода (~150 мс) блокирует event loop.

**Решение:** Запускать в thread pool:
```python
import asyncio
from fastapi.concurrency import run_in_threadpool

qr_base64 = await run_in_threadpool(config_service.generate_qr, uri)
```

---

*Следующий раздел: [[OlcPanel_04_API]] — REST-эндпоинты (вкл. `/api/sub/{token}` с `x-hwid`, `/internal/authz/update`, health), коды ошибок и Pydantic-схемы.*

════════════════════════════════════════════════════════════════════════════════
<!-- Конец файла 03_Архитектура_сервисов.md -->
════════════════════════════════════════════════════════════════════════════════
