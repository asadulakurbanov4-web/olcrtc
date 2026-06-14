════════════════════════════════════════════════════════════════════════════════
<!-- ФАЙЛ 5/10: 04_API.md -->
════════════════════════════════════════════════════════════════════════════════

# OlcPanel — Раздел 4: REST API
## Эндпоинты · Схемы · Коды ответов · Примеры · Pydantic-модели

> **Версия:** 2.0 | **Статус:** 🟡 В разработке
> **Base URL:** `https://{your-domain}/api`
> **Аутентификация:** JWT в httpOnly cookie `access_token`
> **Content-Type:** `application/json` (все запросы и ответы)
> **Зависимости:** [[03_Архитектура_сервисов]], [[02_Модель_данных]]

---

## 4.1 Общие соглашения

### 4.1.1 Структура ответа

Все ответы API придерживаются единой структуры:

```json
// Успешный ответ — данные напрямую (не обёрнуты в {data: ...})
// Исключение: списки возвращают объект с pagination

// Одиночный объект:
{ "id": 1, "name": "Антон", ... }

// Список с пагинацией:
{
  "items": [...],
  "total": 14,
  "skip": 0,
  "limit": 50
}

// Ошибка (всегда):
{
  "detail": "Client not found",
  "code": "CLIENT_NOT_FOUND"    // машиночитаемый код (опционально)
}
```

### 4.1.2 HTTP-коды ответов

| Код | Когда используется |
|-----|--------------------|
| `200 OK` | Успешный GET, PATCH |
| `201 Created` | Успешный POST (создание ресурса) |
| `204 No Content` | Успешный DELETE |
| `400 Bad Request` | Невалидные данные запроса (Pydantic validation error) |
| `401 Unauthorized` | Нет токена или токен невалиден/истёк |
| `403 Forbidden` | Токен валиден, но недостаточно прав |
| `404 Not Found` | Ресурс не найден |
| `409 Conflict` | Конфликт данных (например, username уже занят) |
| `422 Unprocessable Entity` | FastAPI validation error (Pydantic) |
| `429 Too Many Requests` | Rate limit превышен |
| `500 Internal Server Error` | Непредвиденная ошибка сервера |

### 4.1.3 Аутентификация

Все эндпоинты кроме `POST /api/auth/login` и `GET /api/health`
требуют валидный JWT в cookie:

```
Cookie: access_token=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

При отсутствии или истечении токена → `401 Unauthorized`.

Cookie параметры:
- `HttpOnly` — недоступна JavaScript
- `Secure` — только HTTPS
- `SameSite=Strict` — защита от CSRF
- `Path=/api` — только для API-запросов
- `Max-Age=86400` — 24 часа (совпадает с JWT expire)

### 4.1.4 Rate Limiting

| Эндпоинт | Лимит |
|----------|-------|
| `POST /api/auth/login` | 5 запросов / 15 минут / IP |
| `POST /api/*/` (создание) | 30 запросов / минуту / IP |
| Все остальные `/api/*` | 100 запросов / минуту / IP |
| `GET /api/health` | Без лимита |

При превышении:
```
HTTP 429 Too Many Requests
Retry-After: 847
Content-Type: application/json

{"detail": "Too many requests", "retry_after_seconds": 847}
```

### 4.1.5 Pydantic-схемы: базовые типы

```python
# app/schemas/common.py

from pydantic import BaseModel, Field
from datetime import datetime
from typing import Generic, TypeVar

T = TypeVar("T")

class PaginatedResponse(BaseModel, Generic[T]):
    items: list[T]
    total: int
    skip: int
    limit: int

class MessageResponse(BaseModel):
    message: str
    success: bool = True
```

---

## 4.2 Модуль AUTH — Аутентификация

### `POST /api/auth/login`

**Описание:** Вход администратора. Возвращает JWT в httpOnly cookie.

**Аутентификация:** Не требуется

**Rate limit:** 5 попыток / 15 мин / IP

**Request Body:**
```json
{
  "username": "admin",
  "password": "your-secure-password"
}
```

**Pydantic схемы:**
```python
# app/schemas/auth.py

from pydantic import BaseModel, Field, field_validator

class LoginRequest(BaseModel):
    username: str = Field(
        min_length=3,
        max_length=64,
        description="Логин администратора"
    )
    password: str = Field(
        min_length=8,
        max_length=128,
        description="Пароль администратора"
    )

    @field_validator("username")
    @classmethod
    def username_alphanumeric(cls, v: str) -> str:
        if not v.replace("_", "").replace("-", "").isalnum():
            raise ValueError("Username must contain only letters, numbers, _ or -")
        return v.lower()


class TokenResponse(BaseModel):
    token_type: str = "bearer"
    expires_in: int = Field(description="Секунд до истечения токена")
    admin_username: str


class AdminInfo(BaseModel):
    id: int
    username: str
    created_at: datetime
    last_login: datetime | None

    class Config:
        from_attributes = True
```

**Успешный ответ `200 OK`:**
```json
{
  "token_type": "bearer",
  "expires_in": 86400,
  "admin_username": "admin"
}
```
+ `Set-Cookie: access_token=<jwt>; HttpOnly; Secure; SameSite=Strict; Path=/api; Max-Age=86400`

**Ошибки:**
```json
// 401 — неверный логин или пароль (одно сообщение для обоих случаев — prevent user enumeration)
{"detail": "Invalid credentials"}

// 429 — слишком много попыток
{"detail": "Too many login attempts. Try again in 847 seconds.", "retry_after_seconds": 847}
```

**Пример curl:**
```bash
curl -X POST https://panel.example.com/api/auth/login \
  -H "Content-Type: application/json" \
  -c cookies.txt \
  -d '{"username": "admin", "password": "your-password"}'
```

---

### `POST /api/auth/logout`

**Описание:** Выход. Добавляет текущий JWT в revocation list. Очищает cookie.

**Аутентификация:** Требуется

**Request Body:** Пустой

**Успешный ответ `200 OK`:**
```json
{"message": "Logged out successfully", "success": true}
```
+ `Set-Cookie: access_token=; Max-Age=0` (очищает cookie)

**Пример curl:**
```bash
curl -X POST https://panel.example.com/api/auth/logout \
  -b cookies.txt
```

---

### `GET /api/auth/me`

**Описание:** Получить информацию о текущем авторизованном администраторе.

**Аутентификация:** Требуется

**Успешный ответ `200 OK`:**
```json
{
  "id": 1,
  "username": "admin",
  "created_at": "2026-06-01T12:00:00Z",
  "last_login": "2026-06-12T14:32:11Z"
}
```

---

### `POST /api/auth/change-password`

**Описание:** Смена пароля администратора. Инвалидирует все текущие сессии.

**Аутентификация:** Требуется

**Request Body:**
```json
{
  "current_password": "old-password",
  "new_password": "new-strong-password-123",
  "new_password_confirm": "new-strong-password-123"
}
```

**Pydantic схема:**
```python
class ChangePasswordRequest(BaseModel):
    current_password: str = Field(min_length=1)
    new_password: str = Field(min_length=12, max_length=128)
    new_password_confirm: str

    @model_validator(mode="after")
    def passwords_match(self) -> "ChangePasswordRequest":
        if self.new_password != self.new_password_confirm:
            raise ValueError("Passwords do not match")
        return self
```

**Успешный ответ `200 OK`:**
```json
{"message": "Password changed. All sessions invalidated.", "success": true}
```

**Ошибки:**
```json
// 400 — неверный текущий пароль
{"detail": "Current password is incorrect"}

// 400 — пароли не совпадают
{"detail": "Passwords do not match"}
```

---

## 4.3 Модуль CLIENTS — Управление клиентами

### Pydantic схемы клиентов

```python
# app/schemas/client.py

from pydantic import BaseModel, Field, field_validator
from datetime import datetime
from typing import Optional
from enum import Enum

class ClientStatus(str, Enum):
    ACTIVE    = "active"
    SUSPENDED = "suspended"
    EXPIRED   = "expired"
    DELETED   = "deleted"

class ClientCreate(BaseModel):
    name: str = Field(
        min_length=1,
        max_length=128,
        description="Имя клиента для идентификации",
        examples=["Антон"]
    )
    telegram: Optional[str] = Field(
        default=None,
        max_length=128,
        description="Telegram username (@username) или ID",
        examples=["@anton_example"]
    )
    phone: Optional[str] = Field(
        default=None,
        max_length=32,
        description="Номер телефона",
        examples=["+79161234567"]
    )
    notes: Optional[str] = Field(
        default=None,
        max_length=1000,
        description="Заметки администратора"
    )
    plan_id: int = Field(
        description="ID тарифного плана",
        gt=0
    )

    @field_validator("telegram")
    @classmethod
    def validate_telegram(cls, v: str | None) -> str | None:
        if v is None:
            return v
        # Принимаем @username или числовой ID
        if v.startswith("@") and len(v) > 1:
            return v
        if v.isdigit():
            return v
        raise ValueError("Telegram must be @username or numeric ID")


class ClientUpdate(BaseModel):
    name: Optional[str] = Field(default=None, min_length=1, max_length=128)
    telegram: Optional[str] = Field(default=None, max_length=128)
    phone: Optional[str] = Field(default=None, max_length=32)
    notes: Optional[str] = Field(default=None, max_length=1000)


class SubscriptionBrief(BaseModel):
    id: int
    plan_name: str
    status: str
    started_at: datetime
    expires_at: datetime
    days_left: int
    traffic_used_mb: int
    traffic_limit_mb: int
    traffic_used_percent: float

    class Config:
        from_attributes = True


class ConfigBrief(BaseModel):
    id: int
    uri: str
    room_url: str
    status: str
    created_at: datetime

    class Config:
        from_attributes = True


class ClientResponse(BaseModel):
    id: int
    name: str
    telegram: Optional[str]
    phone: Optional[str]
    notes: Optional[str]
    status: ClientStatus
    created_at: datetime
    updated_at: datetime
    active_subscription: Optional[SubscriptionBrief]
    active_config: Optional[ConfigBrief]

    class Config:
        from_attributes = True


class ClientListItem(BaseModel):
    """Облегчённая версия для списка (без деталей конфига)"""
    id: int
    name: str
    telegram: Optional[str]
    status: ClientStatus
    created_at: datetime
    subscription_expires_at: Optional[datetime]
    subscription_days_left: Optional[int]
    traffic_used_percent: Optional[float]
    plan_name: Optional[str]

    class Config:
        from_attributes = True
```

---

### `GET /api/clients`

**Описание:** Список всех клиентов с базовой информацией о подписке.

**Аутентификация:** Требуется

**Query Parameters:**

| Параметр | Тип | По умолчанию | Описание |
|----------|-----|--------------|----------|
| `status` | string | null | Фильтр по статусу: `active`, `suspended`, `expired`, `deleted` |
| `search` | string | null | Поиск по имени, telegram, заметкам |
| `sort_by` | string | `created_at` | Сортировка: `name`, `created_at`, `expires_at`, `traffic_used` |
| `sort_dir` | string | `desc` | Направление: `asc`, `desc` |
| `skip` | integer | `0` | Пропустить N записей |
| `limit` | integer | `50` | Максимум записей (max: 200) |

**Успешный ответ `200 OK`:**
```json
{
  "items": [
    {
      "id": 1,
      "name": "Антон",
      "telegram": "@anton_vpn",
      "status": "active",
      "created_at": "2026-06-01T10:00:00Z",
      "subscription_expires_at": "2026-07-01T10:00:00Z",
      "subscription_days_left": 19,
      "traffic_used_percent": 23.4,
      "plan_name": "Стандарт"
    },
    {
      "id": 2,
      "name": "Мария",
      "telegram": "@masha_m",
      "status": "expired",
      "created_at": "2026-03-15T09:30:00Z",
      "subscription_expires_at": "2026-06-07T09:30:00Z",
      "subscription_days_left": -5,
      "traffic_used_percent": 87.1,
      "plan_name": "Базовый"
    }
  ],
  "total": 14,
  "skip": 0,
  "limit": 50
}
```

**Пример curl:**
```bash
# Только истекающие клиенты
curl -b cookies.txt \
  "https://panel.example.com/api/clients?status=active&sort_by=expires_at&sort_dir=asc"

# Поиск по имени
curl -b cookies.txt \
  "https://panel.example.com/api/clients?search=антон"
```

---

### `POST /api/clients`

**Описание:** Создать нового клиента. Автоматически создаёт конфиг и подписку.

**Аутентификация:** Требуется

**Request Body:**
```json
{
  "name": "Антон",
  "telegram": "@anton_vpn",
  "phone": "+79161234567",
  "notes": "Друг Миши, Москва",
  "plan_id": 3
}
```

**Успешный ответ `201 Created`:**
```json
{
  "id": 5,
  "name": "Антон",
  "telegram": "@anton_vpn",
  "phone": "+79161234567",
  "notes": "Друг Миши, Москва",
  "status": "active",
  "created_at": "2026-06-12T14:00:00Z",
  "updated_at": "2026-06-12T14:00:00Z",
  "active_subscription": {
    "id": 7,
    "plan_name": "Стандарт",
    "status": "active",
    "started_at": "2026-06-12T14:00:00Z",
    "expires_at": "2026-07-12T14:00:00Z",
    "days_left": 30,
    "traffic_used_mb": 0,
    "traffic_limit_mb": 102400,
    "traffic_used_percent": 0.0
  },
  "active_config": {
    "id": 3,
    "uri": "olcrtc://jitsi?datachannel@https://meet1.arbitr.ru/olcrtc-a1b2c3d4#717d93af...",
    "room_url": "https://meet1.arbitr.ru/olcrtc-a1b2c3d4",
    "status": "active",
    "created_at": "2026-06-12T14:00:00Z"
  }
}
```

**Ошибки:**
```json
// 404 — план не найден
{"detail": "Plan not found", "code": "PLAN_NOT_FOUND"}

// 422 — ошибка валидации
{
  "detail": [
    {"loc": ["body", "name"], "msg": "Field required", "type": "missing"},
    {"loc": ["body", "plan_id"], "msg": "Input should be greater than 0", "type": "greater_than"}
  ]
}
```

---

### `GET /api/clients/{client_id}`

**Описание:** Полная карточка клиента.

**Аутентификация:** Требуется

**Path Parameters:** `client_id: int`

**Успешный ответ `200 OK`:**
```json
{
  "id": 1,
  "name": "Антон",
  "telegram": "@anton_vpn",
  "phone": "+79161234567",
  "notes": "Друг Миши, Москва",
  "status": "active",
  "created_at": "2026-06-01T10:00:00Z",
  "updated_at": "2026-06-12T08:00:00Z",
  "active_subscription": {
    "id": 3,
    "plan_name": "Стандарт",
    "status": "active",
    "started_at": "2026-06-01T10:00:00Z",
    "expires_at": "2026-07-01T10:00:00Z",
    "days_left": 19,
    "traffic_used_mb": 23961,
    "traffic_limit_mb": 102400,
    "traffic_used_percent": 23.4
  },
  "active_config": {
    "id": 1,
    "uri": "olcrtc://jitsi?datachannel@https://meet1.arbitr.ru/olcrtc-a1b2c3d4#717d93af...",
    "room_url": "https://meet1.arbitr.ru/olcrtc-a1b2c3d4",
    "status": "active",
    "created_at": "2026-06-01T10:00:00Z"
  }
}
```

**Ошибки:**
```json
// 404
{"detail": "Client not found", "code": "CLIENT_NOT_FOUND"}
```

---

### `PATCH /api/clients/{client_id}`

**Описание:** Обновить данные клиента (имя, telegram, телефон, заметки).

**Аутентификация:** Требуется

**Request Body (все поля опциональны):**
```json
{
  "name": "Антон Иванов",
  "notes": "Обновлённая заметка"
}
```

**Успешный ответ `200 OK`:** Полный объект клиента (как GET)

---

### `DELETE /api/clients/{client_id}`

**Описание:** Мягкое удаление клиента (soft delete). Данные хранятся 90 дней.

**Аутентификация:** Требуется

**Query Parameters:**
- `hard=false` (bool) — жёсткое удаление (только если `deleted_at` > 90 дней назад)

**Успешный ответ `204 No Content`**

**Ошибки:**
```json
// 409 — клиент имеет активную подписку
{
  "detail": "Cannot delete client with active subscription. Suspend or expire first.",
  "code": "CLIENT_HAS_ACTIVE_SUBSCRIPTION"
}
```

---

### `POST /api/clients/{client_id}/suspend`

**Описание:** Приостановить клиента вручную.

**Аутентификация:** Требуется

**Request Body:**
```json
{
  "reason": "Не оплатил за следующий месяц"
}
```

**Pydantic схема:**
```python
class SuspendRequest(BaseModel):
    reason: Optional[str] = Field(default=None, max_length=500)
```

**Успешный ответ `200 OK`:**
```json
{"message": "Client suspended", "success": true, "client_id": 1}
```

---

### `POST /api/clients/{client_id}/restore`

**Описание:** Восстановить приостановленного или удалённого клиента.

**Аутентификация:** Требуется

**Request Body:** Пустой

**Успешный ответ `200 OK`:**
```json
{"message": "Client restored", "success": true, "client_id": 1}
```

**Ошибки:**
```json
// 409 — нет активной подписки для восстановления
{
  "detail": "Client has no active subscription. Extend subscription first.",
  "code": "NO_ACTIVE_SUBSCRIPTION"
}
```

---

## 4.4 Модуль CONFIGS — Конфигурации OlcRTC  🟠 LEGACY (v2.0)

> ⚠️ **LEGACY в v2.0.** Эндпоинты ниже возвращают пер-клиентский `crypto_key`/комнату — отвергнутая модель (Решение 1, [[OlcPanel_00_Введение]]). В v2.0 ключ и комната **общие** (из `server.yaml`), а выдача идёт через **`GET /api/sub/{token}`** и устройства (раздел **4.9**). Поля `crypto_key`/`traffic_*` в ответах — историко-совместимые; для нового кода используйте 4.9–4.12. Trafик-поля (`traffic_used_mb`, `traffic_used_percent`) — **DORMANT** (Решение 5), возвращают `0`/«учёт не настроен».

### Pydantic схемы конфигов

```python
# app/schemas/config.py

from pydantic import BaseModel, Field
from datetime import datetime
from typing import Optional

class ConfigResponse(BaseModel):
    id: int
    client_id: int
    crypto_key: str             # полный 64-hex ключ
    room_name: str
    room_url: str
    jitsi_server: str
    transport: str
    region_label: Optional[str]
    uri: str
    status: str
    created_at: datetime
    revoked_at: Optional[datetime]
    revoke_reason: Optional[str]

    class Config:
        from_attributes = True


class ConfigWithQR(ConfigResponse):
    qr_code_base64: str         # PNG в base64 для отображения в браузере
    yaml_config: str            # полный YAML для OlcBox
    subs_format: str            # sub.md формат с #metadata


class RevokeConfigRequest(BaseModel):
    reason: Optional[str] = Field(
        default=None,
        max_length=500,
        description="Причина отзыва конфига"
    )
    generate_new: bool = Field(
        default=True,
        description="Создать новый конфиг после отзыва"
    )


class ConfigHistoryItem(BaseModel):
    id: int
    status: str
    created_at: datetime
    revoked_at: Optional[datetime]
    revoke_reason: Optional[str]

    class Config:
        from_attributes = True
```

---

### `GET /api/clients/{client_id}/config`

**Описание:** Получить текущий активный конфиг клиента с URI, YAML, QR-кодом и subs-форматом.

**Аутентификация:** Требуется

**Успешный ответ `200 OK`:**
```json
{
  "id": 1,
  "client_id": 1,
  "crypto_key": "717d93af24f732ca2a4ee6e1da03f4f688c9d323548b29ebd8e1c14a3acbf378",
  "room_name": "olcrtc-a1b2c3d4",
  "room_url": "https://meet1.arbitr.ru/olcrtc-a1b2c3d4",
  "jitsi_server": "meet1.arbitr.ru",
  "transport": "datachannel",
  "region_label": "EU/Amsterdam",
  "uri": "olcrtc://jitsi?datachannel@https://meet1.arbitr.ru/olcrtc-a1b2c3d4#717d93af...EU/Amsterdam",
  "status": "active",
  "created_at": "2026-06-01T10:00:00Z",
  "revoked_at": null,
  "revoke_reason": null,
  "qr_code_base64": "iVBORw0KGgoAAAANSUhEUgAAAQAAAAEA...",
  "yaml_config": "mode: cnc\nauth:\n  provider: jitsi\nroom:\n  id: \"https://meet1.arbitr.ru/olcrtc-a1b2c3d4\"\ncrypto:\n  key: \"717d93af...\"\nnet:\n  transport: datachannel\n  dns: \"8.8.8.8:53\"\nsocks:\n  host: \"127.0.0.1\"\n  port: 8808\ndata: data\n",
  "subs_format": "#name: olcpanel-anton-a1b2c3d4\n#update: https://panel.example.com/api/subs/1\n#available: 76gb\n#expires: 2026-07-01T10:00:00Z\nolcrtc://jitsi?datachannel@https://meet1.arbitr.ru/olcrtc-a1b2c3d4#717d93af...\n##name: meet1.arbitr.ru\n##comment: key expires: 2026-07-01 | quota: 100gb, remaining: 76gb | always re-validate\n"
}
```

**Ошибки:**
```json
// 404 — нет активного конфига
{"detail": "No active config found for this client", "code": "NO_ACTIVE_CONFIG"}
```

---

### `POST /api/clients/{client_id}/config/revoke`

**Описание:** Отозвать текущий конфиг и (опционально) создать новый.

**Аутентификация:** Требуется

**Request Body:**
```json
{
  "reason": "Клиент поделился конфигом с посторонними",
  "generate_new": true
}
```

**Успешный ответ `200 OK`:**
```json
{
  "message": "Config revoked and new config generated",
  "success": true,
  "revoked_config_id": 1,
  "new_config": {
    "id": 5,
    "uri": "olcrtc://jitsi?datachannel@https://meet1.arbitr.ru/olcrtc-b5c6d7e8#9f1e2d3c...",
    "qr_code_base64": "iVBORw0KGgoAAAANSUhEUg...",
    "yaml_config": "mode: cnc\n...",
    "subs_format": "#name: olcpanel-anton-b5c6d7e8\n..."
  }
}
```

---

### `GET /api/clients/{client_id}/config/history`

**Описание:** История всех конфигов клиента (активный + отозванные).

**Аутентификация:** Требуется

**Успешный ответ `200 OK`:**
```json
{
  "items": [
    {
      "id": 5,
      "status": "active",
      "created_at": "2026-06-12T10:00:00Z",
      "revoked_at": null,
      "revoke_reason": null
    },
    {
      "id": 1,
      "status": "revoked",
      "created_at": "2026-06-01T10:00:00Z",
      "revoked_at": "2026-06-12T10:00:00Z",
      "revoke_reason": "Клиент поделился конфигом с посторонними"
    }
  ],
  "total": 2,
  "skip": 0,
  "limit": 50
}
```

---

### `GET /api/clients/{client_id}/config/download`

**Описание:** Скачать YAML-конфиг как файл.

**Аутентификация:** Требуется

**Успешный ответ `200 OK`:**
```
Content-Type: text/yaml; charset=utf-8
Content-Disposition: attachment; filename="olcpanel-anton-config.yaml"

mode: cnc
auth:
  provider: jitsi
room:
  id: "https://meet1.arbitr.ru/olcrtc-a1b2c3d4"
crypto:
  key: "717d93af24f732ca2a4ee6e1da03f4f688c9d323548b29ebd8e1c14a3acbf378"
net:
  transport: datachannel
  dns: "8.8.8.8:53"
socks:
  host: "127.0.0.1"
  port: 8808
data: data
```

---

## 4.5 Модуль SUBSCRIPTIONS — Подписки

### Pydantic схемы подписок

```python
# app/schemas/subscription.py

from pydantic import BaseModel, Field, model_validator
from datetime import datetime, date
from typing import Optional
from enum import Enum

class ExtensionDays(int, Enum):
    THIRTY = 30
    SIXTY = 60
    NINETY = 90

class SubscriptionExtend(BaseModel):
    days: Optional[ExtensionDays] = Field(
        default=None,
        description="Стандартный период продления: 30, 60 или 90 дней"
    )
    custom_date: Optional[date] = Field(
        default=None,
        description="Конкретная дата окончания (альтернатива days)"
    )
    amount: Optional[int] = Field(
        default=None,
        ge=0,
        description="Сумма оплаты в рублях (для записи в платежи)"
    )
    payment_method: Optional[str] = Field(
        default="card",
        description="Способ оплаты: card, crypto, cash, other"
    )
    payment_note: Optional[str] = Field(
        default=None,
        max_length=500
    )

    @model_validator(mode="after")
    def validate_extension(self) -> "SubscriptionExtend":
        if self.days is None and self.custom_date is None:
            raise ValueError("Either 'days' or 'custom_date' must be provided")
        if self.days is not None and self.custom_date is not None:
            raise ValueError("Provide either 'days' or 'custom_date', not both")
        return self


class SubscriptionResponse(BaseModel):
    id: int
    client_id: int
    plan_id: int
    plan_name: str
    status: str
    started_at: datetime
    expires_at: datetime
    days_left: int
    traffic_limit_mb: int
    traffic_used_mb: int
    traffic_limit_gb: float
    traffic_used_gb: float
    traffic_used_percent: float
    created_at: datetime
    updated_at: datetime

    class Config:
        from_attributes = True


class SubscriptionHistory(BaseModel):
    items: list[SubscriptionResponse]
    total: int
```

---

### `GET /api/clients/{client_id}/subscription`

**Описание:** Текущая подписка клиента.

**Аутентификация:** Требуется

**Успешный ответ `200 OK`:**
```json
{
  "id": 3,
  "client_id": 1,
  "plan_id": 3,
  "plan_name": "Стандарт",
  "status": "active",
  "started_at": "2026-06-01T10:00:00Z",
  "expires_at": "2026-07-01T10:00:00Z",
  "days_left": 19,
  "traffic_limit_mb": 102400,
  "traffic_used_mb": 23961,
  "traffic_limit_gb": 100.0,
  "traffic_used_gb": 23.4,
  "traffic_used_percent": 23.4,
  "created_at": "2026-06-01T10:00:00Z",
  "updated_at": "2026-06-12T10:05:00Z"
}
```

---

### `POST /api/clients/{client_id}/subscription/extend`

**Описание:** Продлить подписку клиента. Создаёт новую запись в `subscriptions`. Сбрасывает счётчик трафика. Записывает платёж.

**Аутентификация:** Требуется

**Request Body:**
```json
{
  "days": 30,
  "amount": 500,
  "payment_method": "card",
  "payment_note": "Перевод от 12.06, сбербанк"
}
```

Или с конкретной датой:
```json
{
  "custom_date": "2026-08-15",
  "amount": 750,
  "payment_method": "crypto",
  "payment_note": "USDT TRC20"
}
```

**Успешный ответ `200 OK`:**
```json
{
  "message": "Subscription extended",
  "success": true,
  "new_expires_at": "2026-08-01T10:00:00Z",
  "days_added": 30,
  "payment_recorded": true,
  "subscription": {
    "id": 8,
    "status": "active",
    "started_at": "2026-07-01T10:00:00Z",
    "expires_at": "2026-08-01T10:00:00Z",
    "days_left": 50,
    "traffic_used_mb": 0,
    "traffic_limit_mb": 102400,
    "traffic_used_percent": 0.0
  }
}
```

**Ошибки:**
```json
// 404 — клиент не найден
{"detail": "Client not found"}

// 422 — не указан ни days, ни custom_date
{"detail": "Either 'days' or 'custom_date' must be provided"}

// 400 — custom_date в прошлом
{"detail": "custom_date must be in the future"}
```

---

### `GET /api/clients/{client_id}/subscription/history`

**Описание:** История всех подписок клиента.

**Аутентификация:** Требуется

**Успешный ответ `200 OK`:**
```json
{
  "items": [
    {
      "id": 8,
      "plan_name": "Стандарт",
      "status": "active",
      "started_at": "2026-07-01T10:00:00Z",
      "expires_at": "2026-08-01T10:00:00Z",
      "days_left": 50,
      "traffic_used_mb": 0,
      "traffic_limit_mb": 102400,
      "traffic_used_percent": 0.0
    },
    {
      "id": 3,
      "plan_name": "Стандарт",
      "status": "expired",
      "started_at": "2026-06-01T10:00:00Z",
      "expires_at": "2026-07-01T10:00:00Z",
      "days_left": -11,
      "traffic_used_mb": 91234,
      "traffic_limit_mb": 102400,
      "traffic_used_percent": 89.1
    }
  ],
  "total": 2
}
```

---

## 4.6 Модуль PAYMENTS — Платежи

### Pydantic схемы платежей

```python
# app/schemas/payment.py

from pydantic import BaseModel, Field
from datetime import datetime
from typing import Optional
from enum import Enum

class PaymentMethod(str, Enum):
    CARD   = "card"
    CRYPTO = "crypto"
    CASH   = "cash"
    OTHER  = "other"

class PaymentCreate(BaseModel):
    amount: int = Field(ge=0, description="Сумма в рублях")
    method: PaymentMethod = PaymentMethod.CARD
    period_days: int = Field(default=30, ge=1, le=365)
    note: Optional[str] = Field(default=None, max_length=500)

class PaymentResponse(BaseModel):
    id: int
    client_id: int
    subscription_id: Optional[int]
    amount: int
    currency: str
    method: PaymentMethod
    period_days: int
    note: Optional[str]
    created_at: datetime

    class Config:
        from_attributes = True
```

---

### `GET /api/clients/{client_id}/payments`

**Описание:** История платежей клиента.

**Аутентификация:** Требуется

**Успешный ответ `200 OK`:**
```json
{
  "items": [
    {
      "id": 5,
      "client_id": 1,
      "subscription_id": 8,
      "amount": 500,
      "currency": "RUB",
      "method": "card",
      "period_days": 30,
      "note": "Перевод от 12.06, сбербанк",
      "created_at": "2026-06-12T14:00:00Z"
    }
  ],
  "total": 3,
  "total_amount_rub": 1500
}
```

---

### `POST /api/clients/{client_id}/payments`

**Описание:** Записать платёж вручную (без продления подписки).

**Аутентификация:** Требуется

**Request Body:**
```json
{
  "amount": 500,
  "method": "card",
  "period_days": 30,
  "note": "Авансовый платёж"
}
```

**Успешный ответ `201 Created`:** Объект PaymentResponse

---

## 4.7 Модуль TRAFFIC — Статистика трафика

### Pydantic схемы трафика

```python
# app/schemas/traffic.py

from pydantic import BaseModel, Field
from datetime import date, datetime
from typing import Optional

class TrafficDayPoint(BaseModel):
    date: date
    bytes_in: int
    bytes_out: int
    bytes_total: int
    gb_total: float                  # bytes_total / 1024^3, округлено до 2 знаков

class TrafficStatsResponse(BaseModel):
    client_id: int
    period_start: date
    period_end: date
    total_bytes: int
    total_gb: float
    daily: list[TrafficDayPoint]     # точки для графика

class TrafficSummary(BaseModel):
    """Суммарный трафик по всем клиентам"""
    today_bytes: int
    today_gb: float
    month_bytes: int
    month_gb: float
    updated_at: datetime
```

---

### `GET /api/clients/{client_id}/traffic`

**Описание:** Статистика трафика клиента за период.

**Аутентификация:** Требуется

**Query Parameters:**

| Параметр | Тип | По умолчанию | Описание |
|----------|-----|--------------|----------|
| `days` | integer | `30` | Количество дней истории (max: 365) |
| `from_date` | date | null | Начало периода (YYYY-MM-DD) |
| `to_date` | date | null | Конец периода (YYYY-MM-DD) |

**Успешный ответ `200 OK`:**
```json
{
  "client_id": 1,
  "period_start": "2026-05-13",
  "period_end": "2026-06-12",
  "total_bytes": 25132687360,
  "total_gb": 23.41,
  "daily": [
    {
      "date": "2026-06-12",
      "bytes_in": 1073741824,
      "bytes_out": 536870912,
      "bytes_total": 1610612736,
      "gb_total": 1.50
    },
    {
      "date": "2026-06-11",
      "bytes_in": 858993459,
      "bytes_out": 429496730,
      "bytes_total": 1288490189,
      "gb_total": 1.20
    }
  ]
}
```

---

## 4.8 Модуль DASHBOARD — Дашборд

### Pydantic схемы дашборда

```python
# app/schemas/dashboard.py

from pydantic import BaseModel
from datetime import datetime
from typing import Optional
from enum import Enum

class AlertSeverity(str, Enum):
    WARNING = "warning"    # истекает < 3 дней
    DANGER  = "danger"     # истекло

class AlertItem(BaseModel):
    client_id: int
    client_name: str
    telegram: Optional[str]
    severity: AlertSeverity
    message: str                     # "Истекает через 2 дня" / "Истекло 5 дней назад"
    expires_at: datetime
    days_left: int

class ClientCountsByStatus(BaseModel):
    active: int
    suspended: int
    expired: int
    deleted: int
    total: int

class TrafficWidget(BaseModel):
    today_gb: float
    month_gb: float
    chart_labels: list[str]          # ["2026-06-06", ..., "2026-06-12"]
    chart_data_gb: list[float]       # [1.2, 0.8, 2.1, ...]

class OlcRTCStatus(BaseModel):
    status: str                      # "running" / "stopped" / "unknown"
    service_name: str
    uptime_seconds: Optional[int]

class DashboardResponse(BaseModel):
    client_counts: ClientCountsByStatus
    alerts: list[AlertItem]          # истекающие и истёкшие (до 10)
    traffic: TrafficWidget
    olcrtc_status: OlcRTCStatus
    updated_at: datetime
```

---

### `GET /api/dashboard`

**Описание:** Данные для главного дашборда. Все виджеты в одном запросе.

**Аутентификация:** Требуется

**Успешный ответ `200 OK`:**
```json
{
  "client_counts": {
    "active": 12,
    "suspended": 1,
    "expired": 2,
    "deleted": 0,
    "total": 15
  },
  "alerts": [
    {
      "client_id": 3,
      "client_name": "Василий",
      "telegram": "@vasya_v",
      "severity": "warning",
      "message": "Истекает через 2 дня",
      "expires_at": "2026-06-14T10:00:00Z",
      "days_left": 2
    },
    {
      "client_id": 7,
      "client_name": "Мария",
      "telegram": "@masha_m",
      "severity": "danger",
      "message": "Истекло 5 дней назад",
      "expires_at": "2026-06-07T09:30:00Z",
      "days_left": -5
    }
  ],
  "traffic": {
    "today_gb": 4.23,
    "month_gb": 87.41,
    "chart_labels": ["2026-06-06", "2026-06-07", "2026-06-08", "2026-06-09", "2026-06-10", "2026-06-11", "2026-06-12"],
    "chart_data_gb": [11.2, 9.8, 13.4, 8.1, 12.7, 15.3, 4.23]
  },
  "olcrtc_status": {
    "status": "running",
    "service_name": "olcrtc-server",
    "uptime_seconds": 86423
  },
  "updated_at": "2026-06-12T14:05:00Z"
}
```

---

## 4.9 Модуль SETTINGS — Настройки

### Pydantic схемы настроек

```python
# app/schemas/settings.py

from pydantic import BaseModel, Field, HttpUrl
from datetime import datetime
from typing import Optional

class ServerSettingsResponse(BaseModel):
    jitsi_server: str
    default_room_prefix: str
    region_label: str
    transport: str
    dns_server: str
    socks_port: int
    warn_days_before: int
    updated_at: datetime

    class Config:
        from_attributes = True

class ServerSettingsUpdate(BaseModel):
    jitsi_server: Optional[str] = Field(
        default=None,
        description="Хост нового Jitsi-сервера (без https://)",
        examples=["meet2.example.com"]
    )
    region_label: Optional[str] = Field(
        default=None,
        max_length=64
    )
    warn_days_before: Optional[int] = Field(
        default=None,
        ge=1,
        le=30,
        description="За сколько дней предупреждать об истечении"
    )
    dns_server: Optional[str] = Field(
        default=None,
        description="DNS-сервер для клиентов (IP:port)",
        examples=["8.8.8.8:53", "1.1.1.1:53"]
    )
    socks_port: Optional[int] = Field(
        default=None,
        ge=1024,
        le=65535
    )


class PlanResponse(BaseModel):
    id: int
    name: str
    description: Optional[str]
    price: int
    duration_days: int
    traffic_gb: int
    is_active: bool
    sort_order: int

    class Config:
        from_attributes = True


class PlanCreate(BaseModel):
    name: str = Field(min_length=1, max_length=64)
    description: Optional[str] = Field(default=None, max_length=500)
    price: int = Field(ge=0)
    duration_days: int = Field(ge=1, le=365)
    traffic_gb: int = Field(ge=0, description="0 = безлимит")
    sort_order: int = Field(default=0, ge=0)


class PlanUpdate(BaseModel):
    name: Optional[str] = Field(default=None, min_length=1, max_length=64)
    description: Optional[str] = None
    price: Optional[int] = Field(default=None, ge=0)
    duration_days: Optional[int] = Field(default=None, ge=1, le=365)
    traffic_gb: Optional[int] = Field(default=None, ge=0)
    is_active: Optional[bool] = None
    sort_order: Optional[int] = Field(default=None, ge=0)
```

---

### `GET /api/settings`

**Описание:** Текущие настройки сервера OlcRTC и панели.

**Аутентификация:** Требуется

**Успешный ответ `200 OK`:**
```json
{
  "jitsi_server": "meet1.arbitr.ru",
  "default_room_prefix": "olcrtc",
  "region_label": "EU/Amsterdam",
  "transport": "datachannel",
  "dns_server": "8.8.8.8:53",
  "socks_port": 8808,
  "warn_days_before": 3,
  "updated_at": "2026-06-01T12:00:00Z"
}
```

---

### `PATCH /api/settings`

**Описание:** Обновить настройки. При смене `jitsi_server` — перегенерировать конфиги всех активных клиентов и перезапустить OlcRTC.

**Аутентификация:** Требуется

**Request Body:**
```json
{
  "jitsi_server": "meet2.example.com",
  "warn_days_before": 5
}
```

**Успешный ответ `200 OK`:**
```json
{
  "message": "Settings updated",
  "success": true,
  "jitsi_changed": true,
  "configs_regenerated": 12,
  "olcrtc_restarted": true,
  "updated_settings": {
    "jitsi_server": "meet2.example.com",
    "warn_days_before": 5
  }
}
```

---

### `GET /api/settings/plans`

**Описание:** Список всех тарифных планов.

**Аутентификация:** Требуется

**Успешный ответ `200 OK`:**
```json
[
  {"id": 1, "name": "Trial", "price": 0, "duration_days": 7, "traffic_gb": 5, "is_active": true, "sort_order": 0},
  {"id": 2, "name": "Базовый", "price": 300, "duration_days": 30, "traffic_gb": 30, "is_active": true, "sort_order": 1},
  {"id": 3, "name": "Стандарт", "price": 500, "duration_days": 30, "traffic_gb": 100, "is_active": true, "sort_order": 2},
  {"id": 4, "name": "Безлимит", "price": 700, "duration_days": 30, "traffic_gb": 0, "is_active": true, "sort_order": 3}
]
```

---

### `POST /api/settings/plans`

**Описание:** Создать новый тарифный план.

**Request Body:**
```json
{
  "name": "Корпоративный",
  "description": "Для бизнеса, 90 дней, 500 ГБ",
  "price": 1500,
  "duration_days": 90,
  "traffic_gb": 500,
  "sort_order": 4
}
```

**Успешный ответ `201 Created`:** Объект PlanResponse

---

### `PATCH /api/settings/plans/{plan_id}`

**Описание:** Обновить тарифный план. Изменения применяются только к новым подпискам.

**Успешный ответ `200 OK`:** Объект PlanResponse

---

### `POST /api/settings/olcrtc/restart`

**Описание:** Перезапустить OlcRTC-сервер через systemctl.

**Аутентификация:** Требуется

**Успешный ответ `200 OK`:**
```json
{
  "message": "OlcRTC server restarted",
  "success": true,
  "status": "running",
  "restart_time": "2026-06-12T14:10:00Z"
}
```

**Ошибки:**
```json
// 500 — не удалось перезапустить
{
  "detail": "Failed to restart OlcRTC server",
  "code": "OLCRTC_RESTART_FAILED",
  "stderr": "Failed to restart olcrtc-server.service: ..."
}
```

---

### `GET /api/settings/olcrtc/logs`

**Описание:** Последние строки логов OlcRTC-сервера из journald.

**Query Parameters:**
- `lines=100` (integer, max: 500)

**Успешный ответ `200 OK`:**
```json
{
  "lines": [
    "Jun 12 14:00:01 server olcrtc[35919]: rtc: jitsi: MUC joined olcrtc-tunnel-2025; waiting for peer",
    "Jun 12 14:00:15 server olcrtc[35919]: rtc: liveness: bytes=104234 ssn=1 ok",
    "Jun 12 14:05:01 server olcrtc[35919]: rtc: liveness: bytes=98123 ssn=2 ok"
  ],
  "total_lines": 3,
  "service_name": "olcrtc-server"
}
```

---

## 4.10 Модуль HEALTH — Состояние системы

### `GET /api/health`

**Описание:** Базовая проверка работоспособности. Не требует аутентификации. Используется для мониторинга (uptime-сервисы, Nginx health check).

**Аутентификация:** Не требуется

**Успешный ответ `200 OK`:**
```json
{
  "status": "ok",
  "version": "1.0.0",
  "timestamp": "2026-06-12T14:10:00Z"
}
```

**Ошибка `503 Service Unavailable`:**
```json
{
  "status": "error",
  "detail": "Database unavailable"
}
```

---

### `GET /api/health/carriers`

**Описание:** Статус carriers из good-carriers.md и validate-логов. Данные кэшируются 5 минут.

**Аутентификация:** Требуется

**Pydantic схема:**
```python
class CarrierStatus(str, Enum):
    GOLD     = "gold"
    FALLBACK = "fallback"
    DEGRADED = "degraded"
    UNKNOWN  = "unknown"

class CarrierHealth(BaseModel):
    name: str                         # "ct.placetime.team"
    status: CarrierStatus
    is_primary: bool
    last_validated: Optional[datetime]
    soak_kb: Optional[int]            # KB переданных при soak-тесте
    volume_aware: bool
    anonymous: bool
    suspect: bool                     # True если liveness показывает stall
    last_stall_at: Optional[datetime]

class CarriersHealthResponse(BaseModel):
    carriers: list[CarrierHealth]
    updated_at: datetime
    cache_ttl_seconds: int = 300
```

**Успешный ответ `200 OK`:**
```json
{
  "carriers": [
    {
      "name": "ct.placetime.team",
      "status": "gold",
      "is_primary": true,
      "last_validated": "2026-06-12T10:00:00Z",
      "soak_kb": 104,
      "volume_aware": true,
      "anonymous": true,
      "suspect": false,
      "last_stall_at": null
    },
    {
      "name": "mf.example.com",
      "status": "fallback",
      "is_primary": false,
      "last_validated": "2026-06-12T09:30:00Z",
      "soak_kb": 45,
      "volume_aware": false,
      "anonymous": false,
      "suspect": false,
      "last_stall_at": null
    }
  ],
  "updated_at": "2026-06-12T14:05:00Z",
  "cache_ttl_seconds": 300
}
```

---

### `POST /api/health/carriers/refresh`

**Описание:** Принудительно обновить кэш статуса carriers (сбросить TTL и перечитать файлы).

**Аутентификация:** Требуется

**Успешный ответ `200 OK`:**
```json
{
  "message": "Carriers health refreshed",
  "success": true,
  "carriers_count": 2,
  "updated_at": "2026-06-12T14:10:00Z"
}
```

---

## 4.11 Полная таблица всех эндпоинтов

| Метод | Путь | Auth | Описание |
|-------|------|:----:|----------|
| POST | `/api/auth/login` | ❌ | Вход |
| POST | `/api/auth/logout` | ✅ | Выход |
| GET | `/api/auth/me` | ✅ | Текущий администратор |
| POST | `/api/auth/change-password` | ✅ | Смена пароля |
| GET | `/api/clients` | ✅ | Список клиентов |
| POST | `/api/clients` | ✅ | Создать клиента |
| GET | `/api/clients/{id}` | ✅ | Карточка клиента |
| PATCH | `/api/clients/{id}` | ✅ | Обновить клиента |
| DELETE | `/api/clients/{id}` | ✅ | Удалить клиента (soft) |
| POST | `/api/clients/{id}/suspend` | ✅ | Приостановить |
| POST | `/api/clients/{id}/restore` | ✅ | Восстановить |
| GET | `/api/clients/{id}/config` | ✅ | Конфиг + URI + QR + YAML |
| POST | `/api/clients/{id}/config/revoke` | ✅ | Отозвать конфиг |
| GET | `/api/clients/{id}/config/history` | ✅ | История конфигов |
| GET | `/api/clients/{id}/config/download` | ✅ | Скачать YAML |
| GET | `/api/clients/{id}/subscription` | ✅ | Текущая подписка |
| POST | `/api/clients/{id}/subscription/extend` | ✅ | Продлить подписку |
| GET | `/api/clients/{id}/subscription/history` | ✅ | История подписок |
| GET | `/api/clients/{id}/payments` | ✅ | История платежей |
| POST | `/api/clients/{id}/payments` | ✅ | Записать платёж |
| GET | `/api/clients/{id}/traffic` | ✅ | Статистика трафика |
| GET | `/api/dashboard` | ✅ | Данные дашборда |
| GET | `/api/settings` | ✅ | Настройки сервера |
| PATCH | `/api/settings` | ✅ | Обновить настройки |
| GET | `/api/settings/plans` | ✅ | Список тарифов |
| POST | `/api/settings/plans` | ✅ | Создать тариф |
| PATCH | `/api/settings/plans/{id}` | ✅ | Обновить тариф |
| POST | `/api/settings/olcrtc/restart` | ✅ | Перезапустить OlcRTC |
| GET | `/api/settings/olcrtc/logs` | ✅ | Логи OlcRTC |
| GET | `/api/health` | ❌ | Health check (публичный) |
| GET | `/api/health/carriers` | ✅ | Статус carriers |
| POST | `/api/health/carriers/refresh` | ✅ | Обновить статус carriers |

**Итого:** 32 эндпоинта, из которых 30 требуют аутентификации.

---

## 4.9 Модуль SUBSCRIPTION — выдача подписки и привязка устройства (v2.0, публичный)

### `GET /api/sub/{token}` — публичный (без JWT)

**Описание:** отдаёт бандл подписки olcbox по персональному sub-токену (`client.olcrtc_key`). **Ловит заголовок `x-hwid`** (deviceId), привязывает устройство и обновляет allowlist.

```
Заголовки запроса:
  x-hwid: <deviceId>        # olcbox присылает при загрузке/ревалидации

Поведение:
  1. token → client; если не найден/ротирован → 404 (старая ссылка рвётся)
  2. если x-hwid задан → bind_device(client_id, x-hwid):
        UPSERT user_devices(device_id=x-hwid, server_id=subscription.server_id,
                            is_active = (подписка active))
        → authz_writer.write_authz_file(db)  (атомарно, version++)
        → (флот) Coordinator.on_change(server_id) → push
  3. вернуть бандл: общая комната+ключ из server.yaml (generate_subscription_bundle)

Ответ 200 (text/plain, формат sub):
  #name: olcpanel-anton
  #update: https://panel.example.com/api/sub/{token}
  #expires: 2026-07-01T10:00:00Z
  olcrtc://jitsi?datachannel@https://meet1.arbitr.ru/olcrtc-tunnel-prod-mf-fb#<shared_key>
  ##comment: expires: 2026-07-01 | re-validate recommended

Коды: 200 OK | 404 (токен не найден/ротирован)
```

> Это единственная точка, где `deviceId` попадает в систему. Заблокированная/истёкшая подписка: токен может вернуть бандл (или 404 — по политике), но устройство **не** добавляется в `allow` (`is_active=false`). Реальная блокировка — на `Gate`, не на отдаче файла.

### `POST /api/clients/{client_id}/sub/rotate` — ротация sub-токена (JWT)

```
Генерирует новый client.olcrtc_key; старый /api/sub/{old} → 404.
НЕ меняет общий ключ и НЕ трогает активные user_devices.
Ответ 200: { "sub_url": "https://.../api/sub/<new>", "qr_base64": "..." }
```

## 4.10 Модуль DEVICES — устройства клиента (v2.0, JWT)

### `GET /api/clients/{client_id}/devices`
```
200: [{ "id":1, "device_id":"hwid-aaa", "label":"телефон",
         "server_id":1, "is_active":true, "last_used_at":"2026-06-14T08:00:00Z" }]
```

### `DELETE /api/clients/{client_id}/devices/{device_id}`
```
Удаляет/деактивирует устройство → убирает device_id из allow:
  → authz_writer.write_authz_file(db) (version++) → (флот) push
204 No Content. Остальные клиенты (общий ключ) НЕ затронуты.
```

### `POST /api/clients/{client_id}/devices/{device_id}/move`  (Этап 3)
```
Тело: { "to_server_id": 2 }
Миграция устройства между серверами (порядок 4.7: сначала add на новый,
потом remove со старого — fail-safe). 200 после подтверждения обоих push.
```

## 4.11 Модуль SERVERS / FLEET — реестр серверов (v2.0, JWT)

### `GET /api/servers`
```
200: [{
  "id":1, "name":"default", "host":"localhost", "status":"active", "priority":100,
  "fail_mode":"lkg",
  "expected_authz_version":42, "applied_authz_version":42,   // совпали — ок
  "lkg_valid_at":"2026-06-14T08:51:00Z", "load_errors":0,
  "last_health_at":"2026-06-14T08:52:00Z"
}]
```
> `applied < expected` ИЛИ `load_errors>0` ИЛИ устаревший `lkg_valid_at` ⇒ сервер «Требует внимания» (5.3) — рендерится на дашборде ([[OlcPanel_06_UI_Карта_экранов]]).

### `POST /api/servers` · `PATCH /api/servers/{id}` · `DELETE /api/servers/{id}`
```
CRUD реестра (name, host, port, api_base, priority, status, fail_mode).
На MVP — одна строка 'default'; контракт закладывается с Этапа 1.
```

### `POST /api/servers/{id}/repush-authz` — ручная пере-доставка (JWT)
```
Принудительный пересчёт среза allow для server_id и push (best-effort).
Используется при load_errors>0 / расхождении версий. 202 Accepted.
```

## 4.12 INTERNAL — доставка authz и health (сервер↔панель)

### `POST /internal/authz/update` — приём allowlist сервером (вариант B, Этап 3)
**Эндпоинт на стороне olcrtc-сервера**, не панели. Защита: `mTLS`/подписанный токен.

```
Тело (payload authz.json):
  { "version":43, "updated_at":"...", "mode":"allowlist",
    "allow":["hwid-aaa"], "deny":[] }

Сервер:
  1. проверяет подпись/mTLS (иначе 401)
  2. ОТВЕРГАЕТ version ≤ applied_authz_version (иначе откат) → 409 Conflict
  3. валидирует схему (иначе 400, LKG не трогается)
  4. пишет authz.json АТОМАРНО (tmp+fsync+rename)
  5. отвечает 200 ТОЛЬКО после успешной записи  → панель ставит expected=43

Коды: 200 (записано) | 400 (битый payload) | 401 (подпись) | 409 (version ≤ applied)
```

### `GET /health` — статус сервера (для ServerHealthService)
**Эндпоинт на стороне olcrtc-сервера.** Источник health-полей `servers`.

```
200: {
  "service":"up",
  "authz": {
    "mode":"allowlist",
    "fail_mode":"lkg",
    "applied_version":43,           // что Gate реально применил
    "lkg_valid_at":"2026-06-14T08:51:00Z",
    "load_errors":0,                // >0 ⇒ Gate работает по LKG из-за битого файла
    "allow_count":12,
    "stale": false                  // now - lkg_valid_at > lkg_max_age ?
  }
}
```
> `stale:true` или `load_errors>0` означает: сервер жив, но применяет **устаревший-но-валидный** allowlist (последствие last-known-good, 5.2). Панель эскалирует в «Требуют внимания» + alert.

---

*Следующий раздел: [[OlcPanel_05_Бизнес_логика]] — алгоритмы генерации URI, учёта трафика,
жизненного цикла подписки, отключения клиента*

════════════════════════════════════════════════════════════════════════════════
<!-- Конец файла 04_API.md -->
════════════════════════════════════════════════════════════════════════════════
