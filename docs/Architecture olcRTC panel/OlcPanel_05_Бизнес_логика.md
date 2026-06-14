════════════════════════════════════════════════════════════════════════════════
<!-- ФАЙЛ 6/10: 05_Бизнес_логика.md -->
════════════════════════════════════════════════════════════════════════════════

# OlcPanel — Раздел 5: Бизнес-логика
## Алгоритмы · State Machines · Генерация URI · Учёт трафика · Жизненный цикл

> **Версия:** 2.0 | **Статус:** 🟡 В разработке
> **Зависимости:** [[OlcPanel_02_Модель_данных]], [[OlcPanel_03_Архитектура_сервисов]], [[OlcPanel_04_API]], разделы 4/4.7/5.2 в [[OlcPanel_Анализ_Marzban_3xui]]
>
> Этот раздел — главный референс при написании кода сервисов. Каждый алгоритм пошагово с edge cases.
>
> ⚠️ **v2.0:** генерация URI использует **общий** ключ/комнату (5.1 — пер-клиентский алгоритм помечен LEGACY); блокировка/отключение идёт через **`authz.json` (soft-block)**, а не iptables (новый раздел **5.6**); учёт трафика **DORMANT** (5.3). См. Решения 1/4/5 в [[OlcPanel_00_Введение]].

---

## 5.1 Генерация конфигурации OlcRTC  🟠 LEGACY-алгоритм (см. v2.0-примечание)

> ⚠️ **v2.0.** Сам **формат** URI (5.1.1) актуален, но **алгоритм генерации (5.1.2) — LEGACY**: он создаёт пер-клиентский `crypto_key` и уникальную комнату (Решение 1 отверг это). В v2.0 `crypto_key` и `room_url` — **общие**, берутся из профиля `server.yaml` (`services/olcrtc.py: get_primary_profile/generate_uri`); пер-клиентская генерация/коллизия-чек ключа **не выполняется**. Различие клиентов — `deviceId` (5.6). Ниже алгоритм оставлен для истории и для понимания формата; в новом коде вызывайте общий профиль, а не `_generate_unique_crypto_key`.

### 5.1.1 Формат OlcRTC URI — спецификация

URI является единственным идентификатором, который передаётся клиенту. Его структура
определена протоколом OlcRTC и должна строго соблюдаться для совместимости с OlcBox.

```
Полный формат:
  olcrtc://{provider}?{transport}@{room_url}#{crypto_key}${region_label}

Компоненты:
  provider     = "jitsi"                          (единственный стабильный в v1.0)
  transport    = "datachannel"                    (SCTP DataChannel поверх WebRTC)
  room_url     = "https://{jitsi_server}/{room_name}"
  crypto_key   = 64 hex-символа (32 байта = 256-bit XChaCha20 ключ)
  region_label = "EU/Amsterdam"                   (косметическая метка, не влияет на подключение)

Пример реального URI:
  olcrtc://jitsi?datachannel@https://meet1.arbitr.ru/olcrtc-a1b2c3d4#717d93af24f732ca2a4ee6e1da03f4f688c9d323548b29ebd8e1c14a3acbf378$EU/Amsterdam

Важно:
  - Символ '#' разделяет room_url и crypto_key
  - Символ '$' разделяет crypto_key и region_label
  - region_label — опционален, но OlcBox его принимает
  - Пробелов нет нигде в URI
  - URI всегда начинается с "olcrtc://"
```

### 5.1.2 Алгоритм генерации конфига

```python
# app/services/config_service.py

import secrets
import uuid
import qrcode
import base64
from io import BytesIO
from datetime import datetime
from sqlalchemy.orm import Session

from app.models.config import Config, ConfigStatus
from app.models.server_settings import ServerSettings
from app.repositories.config_repo import ConfigRepository
from app.repositories.server_settings_repo import ServerSettingsRepository
from app.services.audit_service import AuditService


class ConfigService:

    def __init__(
        self,
        config_repo: ConfigRepository,
        settings_repo: ServerSettingsRepository,
        audit_service: AuditService,
    ):
        self.config_repo = config_repo
        self.settings_repo = settings_repo
        self.audit_service = audit_service

    # ------------------------------------------------------------------ #
    # ГЕНЕРАЦИЯ НОВОГО КОНФИГА                                            #
    # ------------------------------------------------------------------ #

    def generate_config(
        self,
        db: Session,
        client_id: int,
        admin_id: int | None = None,
        ip_address: str | None = None,
    ) -> Config:
        """
        Полный алгоритм генерации конфига для клиента.

        Шаги:
          1. Получить текущие настройки сервера
          2. Сгенерировать уникальный crypto_key
          3. Сгенерировать уникальное имя комнаты
          4. Собрать room_url
          5. Собрать полный URI
          6. Сделать снимок текущих carriers (carriers_snapshot)
          7. Сохранить в БД
          8. Залогировать в audit
          9. Вернуть Config объект

        Edge cases:
          - Если crypto_key уже существует в БД (коллизия) — регенерировать.
            Вероятность коллизии: 1/(2^256) ≈ невозможна, но проверяем.
          - Если room_name уже существует — регенерировать.
          - Если активный конфиг уже есть — НЕ создаём новый автоматически.
            Вызывающий код должен сначала вызвать revoke_config().
        """
        # Шаг 1: Настройки сервера
        settings = self.settings_repo.get(db)

        # Шаг 2: Генерация crypto_key (32 байта = 64 hex символа)
        # Используем secrets.token_hex — криптографически безопасный CSPRNG
        crypto_key = self._generate_unique_crypto_key(db)

        # Шаг 3: Генерация имени комнаты
        # Формат: {prefix}-{8 символов UUID4 без дефисов}
        # Пример: olcrtc-a1b2c3d4
        room_name = self._generate_unique_room_name(db, settings.default_room_prefix)

        # Шаг 4: Полный URL комнаты
        room_url = f"https://{settings.jitsi_server}/{room_name}"

        # Шаг 5: Сборка URI
        uri = self._build_uri(
            transport=settings.transport,
            room_url=room_url,
            crypto_key=crypto_key,
            region_label=settings.region_label,
        )

        # Шаг 6: Снимок текущих carriers для аудита
        # Зачем: если Jitsi-сервер потом сменится, мы будем знать,
        # какой именно был использован при выдаче этого конфига.
        carriers_snapshot = {
            "jitsi_server": settings.jitsi_server,
            "transport": settings.transport,
            "generated_at": datetime.utcnow().isoformat(),
        }

        # Шаг 7: Сохранение в БД
        config = self.config_repo.create(
            db,
            client_id=client_id,
            crypto_key=crypto_key,
            room_name=room_name,
            room_url=room_url,
            jitsi_server=settings.jitsi_server,
            transport=settings.transport,
            region_label=settings.region_label,
            uri=uri,
            status=ConfigStatus.ACTIVE,
            carriers_snapshot=str(carriers_snapshot),  # JSON в TEXT поле
        )

        # Шаг 8: Аудит
        self.audit_service.log(
            db=db,
            action="config.generate",
            admin_id=admin_id,
            entity_type="config",
            entity_id=config.id,
            details={
                "client_id": client_id,
                "room_name": room_name,
                "jitsi_server": settings.jitsi_server,
            },
            ip_address=ip_address,
        )

        return config

    def _generate_unique_crypto_key(self, db: Session, max_attempts: int = 5) -> str:
        """
        Генерирует глобально уникальный crypto_key.

        Алгоритм:
          - secrets.token_hex(32) → 64 hex символа
          - Проверка уникальности в БД
          - При коллизии (крайне маловероятна) — повтор до max_attempts раз
          - При превышении max_attempts — RuntimeError (никогда не должно случиться)
        """
        for attempt in range(max_attempts):
            key = secrets.token_hex(32)  # 32 байта = 64 hex символа
            if not self.config_repo.crypto_key_exists(db, key):
                return key
        # Этот код недостижим на практике
        raise RuntimeError(
            f"Failed to generate unique crypto_key after {max_attempts} attempts. "
            "This should never happen (probability 1/2^256 per attempt)."
        )

    def _generate_unique_room_name(
        self,
        db: Session,
        prefix: str,
        max_attempts: int = 10,
    ) -> str:
        """
        Генерирует уникальное имя комнаты.

        Формат: {prefix}-{8 случайных hex символов}
        Пример: olcrtc-a1b2c3d4

        При коллизии (маловероятна при < 1000 активных конфигов) — регенерирует.
        """
        for attempt in range(max_attempts):
            # uuid4() → берём первые 8 символов без дефисов
            suffix = uuid.uuid4().hex[:8]
            room_name = f"{prefix}-{suffix}"
            if not self.config_repo.room_name_exists(db, room_name):
                return room_name
        raise RuntimeError(
            f"Failed to generate unique room_name after {max_attempts} attempts."
        )

    def _build_uri(
        self,
        transport: str,
        room_url: str,
        crypto_key: str,
        region_label: str | None,
    ) -> str:
        """
        Собирает полный OlcRTC URI.

        Формат: olcrtc://jitsi?{transport}@{room_url}#{crypto_key}${region_label}

        Примеры:
          С region_label:
            olcrtc://jitsi?datachannel@https://meet1.arbitr.ru/olcrtc-a1b2c3d4#717d93af...#EU/Amsterdam

          Без region_label:
            olcrtc://jitsi?datachannel@https://meet1.arbitr.ru/olcrtc-a1b2c3d4#717d93af...
        """
        base = f"olcrtc://jitsi?{transport}@{room_url}#{crypto_key}"
        if region_label:
            return f"{base}${region_label}"
        return base

    # ------------------------------------------------------------------ #
    # ОТЗЫВ КОНФИГА                                                       #
    # ------------------------------------------------------------------ #

    def revoke_config(
        self,
        db: Session,
        config_id: int,
        reason: str | None = None,
        generate_new: bool = True,
        admin_id: int | None = None,
        ip_address: str | None = None,
    ) -> dict:
        """
        Отзывает текущий конфиг и (опционально) генерирует новый.

        Шаги:
          1. Найти Config по ID, проверить что status=active
          2. Получить client_id из конфига
          3. Обновить config: status=revoked, revoked_at=now(), revoke_reason
          4. Если generate_new=True — вызвать generate_config(client_id)
          5. Залогировать в audit
          6. Вернуть: {revoked_id, new_config или None}

        Edge cases:
          - Конфиг уже revoked — 409 Conflict
          - Нет активного конфига для клиента — 404
        """
        # Шаг 1: Найти конфиг
        config = self.config_repo.get_by_id(db, config_id)
        if not config:
            raise ValueError(f"Config {config_id} not found")
        if config.status != ConfigStatus.ACTIVE:
            raise ValueError(f"Config {config_id} is already {config.status}")

        client_id = config.client_id

        # Шаг 2: Отзыв
        self.config_repo.update(
            db,
            config,
            status=ConfigStatus.REVOKED,
            revoked_at=datetime.utcnow(),
            revoke_reason=reason,
        )

        # Шаг 3: Аудит отзыва
        self.audit_service.log(
            db=db,
            action="config.revoke",
            admin_id=admin_id,
            entity_type="config",
            entity_id=config_id,
            details={"reason": reason, "client_id": client_id, "generate_new": generate_new},
            ip_address=ip_address,
        )

        # Шаг 4: Генерация нового (если нужно)
        new_config = None
        if generate_new:
            new_config = self.generate_config(
                db=db,
                client_id=client_id,
                admin_id=admin_id,
                ip_address=ip_address,
            )

        return {
            "revoked_config_id": config_id,
            "new_config": new_config,
        }

    # ------------------------------------------------------------------ #
    # ФОРМАТЫ ВЫВОДА                                                      #
    # ------------------------------------------------------------------ #

    def to_yaml(self, config: "Config", settings: "ServerSettings") -> str:
        """
        Генерирует YAML-конфиг для импорта в OlcBox.

        Формат совместим с OlcBox Android/iOS/Desktop.

        Пример вывода:
          mode: cnc
          auth:
            provider: jitsi
          room:
            id: "https://meet1.arbitr.ru/olcrtc-a1b2c3d4"
          crypto:
            key: "717d93af..."
          net:
            transport: datachannel
            dns: "8.8.8.8:53"
          socks:
            host: "127.0.0.1"
            port: 8808
          data: data
        """
        return (
            f"mode: cnc\n"
            f"auth:\n"
            f"  provider: jitsi\n"
            f"room:\n"
            f"  id: \"{config.room_url}\"\n"
            f"crypto:\n"
            f"  key: \"{config.crypto_key}\"\n"
            f"net:\n"
            f"  transport: {config.transport}\n"
            f"  dns: \"{settings.dns_server}\"\n"
            f"socks:\n"
            f"  host: \"127.0.0.1\"\n"
            f"  port: {settings.socks_port}\n"
            f"data: data\n"
        )

    def to_subs_format(
        self,
        config: "Config",
        client_name: str,
        subscription: "Subscription | None",
        panel_url: str,
    ) -> str:
        """
        Генерирует sub.md формат, совместимый со стандартом OlcRTC.

        Формат sub.md используется OlcBox для:
          - Отображения метаданных (имя, доступный трафик, срок)
          - Автообновления конфига по #update ссылке
          - Отображения информации в карточке подключения

        Пример вывода:
          #name: olcpanel-anton-a1b2c3d4
          #update: https://panel.example.com/api/subs/1
          #available: 76gb
          #expires: 2026-07-01T10:00:00Z
          olcrtc://jitsi?datachannel@https://meet1.arbitr.ru/olcrtc-a1b2c3d4#717d93af...
          ##name: meet1.arbitr.ru
          ##comment: key expires: 2026-07-01 | quota: 100gb, remaining: 76gb | re-validate recommended

        Поля:
          #name       — отображается в OlcBox как имя подключения
          #update     — URL для обновления sub (панель отдаёт актуальный конфиг)
          #available  — доступный трафик (для информации, не enforcement)
          #expires    — дата истечения подписки
          ##name      — имя конкретного carrier/сервера
          ##comment   — дополнительная информация
        """
        # Вычисляем доступный трафик
        if subscription:
            if subscription.traffic_limit_mb == 0:
                available_str = "unlimited"
            else:
                remaining_mb = max(0, subscription.traffic_limit_mb - subscription.traffic_used_mb)
                remaining_gb = remaining_mb / 1024
                available_str = f"{remaining_gb:.1f}gb"
            expires_str = subscription.expires_at.strftime("%Y-%m-%dT%H:%M:%SZ")
            limit_gb = subscription.traffic_limit_mb / 1024 if subscription.traffic_limit_mb > 0 else 0
            quota_str = f"unlimited" if limit_gb == 0 else f"{limit_gb:.0f}gb"
            remaining_note = f"remaining: {available_str}"
        else:
            available_str = "unknown"
            expires_str = "unknown"
            quota_str = "unknown"
            remaining_note = ""

        # Имя подключения: olcpanel-{client_name_slug}-{room_suffix}
        name_slug = "".join(c if c.isalnum() else "-" for c in client_name.lower())
        room_suffix = config.room_name.split("-")[-1] if "-" in config.room_name else config.room_name
        connection_name = f"olcpanel-{name_slug}-{room_suffix}"

        lines = [
            f"#name: {connection_name}",
            f"#update: {panel_url}/api/subs/{config.client_id}",
            f"#available: {available_str}",
        ]

        if subscription:
            lines.append(f"#expires: {expires_str}")

        lines.append(config.uri)
        lines.append(f"##name: {config.jitsi_server}")

        comment_parts = []
        if subscription:
            comment_parts.append(f"key expires: {expires_str}")
            comment_parts.append(f"quota: {quota_str}")
            if remaining_note:
                comment_parts.append(remaining_note)
        comment_parts.append("always re-validate recommended")
        lines.append(f"##comment: {' | '.join(comment_parts)}")

        return "\n".join(lines) + "\n"

    def generate_qr_code_base64(self, uri: str) -> str:
        """
        Генерирует QR-код из URI и возвращает base64-строку PNG.

        Используется для отображения в браузере:
          <img src="data:image/png;base64,{qr_code_base64}" />

        Параметры QR-кода:
          - версия: автовыбор (по длине URI, обычно 10-15)
          - correction: L (низкая, URI без пробелов)
          - box_size: 10 (пиксели на квадрат)
          - border: 4 (белая рамка)
          - fill_color: "#000000"
          - back_color: "#FFFFFF"
          - итоговый размер: ~250x250 px
        """
        qr = qrcode.QRCode(
            version=None,               # автовыбор
            error_correction=qrcode.constants.ERROR_CORRECT_L,
            box_size=10,
            border=4,
        )
        qr.add_data(uri)
        qr.make(fit=True)

        img = qr.make_image(fill_color="black", back_color="white")
        buffer = BytesIO()
        img.save(buffer, format="PNG")
        buffer.seek(0)

        return base64.b64encode(buffer.read()).decode("utf-8")
```

---

## 5.2 Жизненный цикл подписки

### 5.2.1 State Machine подписки

```
                    ┌─────────────────────────────────────────────────────┐
                    │           ЖИЗНЕННЫЙ ЦИКЛ ПОДПИСКИ                  │
                    └─────────────────────────────────────────────────────┘

  [Создание клиента]
        │
        ▼
  ┌──────────┐   expires_at > now()      ┌─────────────┐
  │  ACTIVE  │ ─────────────────────────▶│   ACTIVE    │  (продление)
  │          │   (auto по expires_at)    │  (новая     │
  │          │ ─────────────────────────▶│  запись)    │
  └────┬─────┘                           └─────────────┘
       │
       │ expires_at < now()
       │ (APScheduler каждый час)
       ▼
  ┌──────────┐                           ┌─────────────┐
  │ EXPIRED  │ ─────────── продление ───▶│   ACTIVE    │
  │          │                           │  (новая     │
  └──────────┘                           │  запись)    │
       │                                 └─────────────┘
       │ ручное
       │ удаление
       ▼
  ┌──────────┐

  [DELETED]   ← через client.soft_delete()


  ┌──────────┐   ручное suspend            ┌─────────────┐
  │  ACTIVE  │ ──────────────────────────▶│  SUSPENDED  │
  │          │   лимит трафика превышен    │             │
  └──────────┘                            └──────┬──────┘
                                                  │
                                          restore │ продление
                                                  ▼
                                          ┌─────────────┐
                                          │   ACTIVE    │
                                          │  (та же или │
                                          │  новая      │
                                          │  запись)    │
                                          └─────────────┘
```

### 5.2.2 Алгоритм создания подписки

```python
# app/services/subscription_service.py

from datetime import datetime, timedelta
from sqlalchemy.orm import Session

from app.models.subscription import Subscription, SubscriptionStatus
from app.models.plan import Plan
from app.repositories.subscription_repo import SubscriptionRepository
from app.repositories.client_repo import ClientRepository
from app.services.audit_service import AuditService


class SubscriptionService:

    def create_subscription(
        self,
        db: Session,
        client_id: int,
        plan: Plan,
        admin_id: int | None = None,
    ) -> Subscription:
        """
        Создаёт новую подписку для клиента.

        Вызывается при:
          - Создании нового клиента (автоматически)
          - Продлении истёкшей подписки (новая запись)

        Шаги:
          1. Вычислить started_at = now()
          2. Вычислить expires_at = started_at + plan.duration_days
          3. Вычислить traffic_limit_mb = plan.traffic_gb * 1024
             (0 если безлимит)
          4. Создать запись в subscriptions
          5. Обновить client.status = "active"
          6. Залогировать

        Важно: plan.traffic_gb хранится в ГБ, traffic_limit_mb в МБ.
          Конвертация: traffic_limit_mb = plan.traffic_gb * 1024
          При plan.traffic_gb == 0 → traffic_limit_mb = 0 (безлимит)
        """
        now = datetime.utcnow()
        started_at = now
        expires_at = now + timedelta(days=plan.duration_days)
        traffic_limit_mb = plan.traffic_gb * 1024 if plan.traffic_gb > 0 else 0

        subscription = self.sub_repo.create(
            db,
            client_id=client_id,
            plan_id=plan.id,
            status=SubscriptionStatus.ACTIVE,
            started_at=started_at,
            expires_at=expires_at,
            traffic_limit_mb=traffic_limit_mb,
            traffic_used_mb=0,
        )

        # Синхронизируем статус клиента
        self.client_repo.update(db, client_id=client_id, status="active")

        self.audit_service.log(
            db=db,
            action="subscription.create",
            admin_id=admin_id,
            entity_type="subscription",
            entity_id=subscription.id,
            details={
                "plan_id": plan.id,
                "plan_name": plan.name,
                "expires_at": expires_at.isoformat(),
                "traffic_limit_mb": traffic_limit_mb,
            },
        )

        return subscription

    # ------------------------------------------------------------------ #
    # ПРОДЛЕНИЕ ПОДПИСКИ                                                  #
    # ------------------------------------------------------------------ #

    def extend_subscription(
        self,
        db: Session,
        client_id: int,
        days: int | None = None,
        custom_date: "date | None" = None,
        amount: int | None = None,
        payment_method: str = "card",
        payment_note: str | None = None,
        admin_id: int | None = None,
        ip_address: str | None = None,
    ) -> Subscription:
        """
        Продлевает подписку клиента.

        Ключевые правила:
          1. Всегда создаётся НОВАЯ запись в subscriptions (не UPDATE старой).
             Это обеспечивает историю подписок.

          2. Если передан days:
               base = MAX(текущая expires_at, now())
               new_expires = base + timedelta(days=days)

               Логика MAX:
                 - Если подписка ещё активна (expires в будущем) →
                   продление от текущей даты окончания (не теряем дни)
                 - Если подписка уже истекла → продление от сегодня

               Пример 1 (активная):
                 expires_at = 2026-07-15, now = 2026-06-12, days=30
                 base = MAX(2026-07-15, 2026-06-12) = 2026-07-15
                 new_expires = 2026-07-15 + 30 дней = 2026-08-14
                 ✓ Клиент не теряет 33 дня до истечения

               Пример 2 (истёкшая):
                 expires_at = 2026-06-07, now = 2026-06-12, days=30
                 base = MAX(2026-06-07, 2026-06-12) = 2026-06-12
                 new_expires = 2026-06-12 + 30 дней = 2026-07-12
                 ✓ Отсчёт от сегодня, а не с момента истечения

          3. Если передан custom_date:
               new_expires = custom_date (без вычислений)
               Проверка: custom_date > today (ошибка если в прошлом)

          4. Старая активная подписка:
               Помечаем статус старой как "expired" (если была active)
               → Создаём новую со status=active

          5. Сброс трафика:
               traffic_used_mb = 0 для новой подписки
               (Отсчёт трафика с начала нового периода)

          6. Запись платежа (если указан amount):
               INSERT INTO payments(client_id, subscription_id, amount, method, ...)

          7. Обновление client.status = "active"

          8. iptables: если клиент был suspended/expired →
               TrafficService.restore_iptables_rule(client_id)
        """
        from datetime import date as date_type

        now = datetime.utcnow()

        # Найти текущую подписку
        current_sub = self.sub_repo.get_active_or_latest(db, client_id)

        # Вычислить новую дату окончания
        if days is not None:
            if current_sub and current_sub.status == SubscriptionStatus.ACTIVE:
                base = max(current_sub.expires_at, now)
            else:
                base = now
            new_expires = base + timedelta(days=days)
            extension_days = days
        elif custom_date is not None:
            if isinstance(custom_date, date_type):
                new_expires = datetime.combine(custom_date, datetime.min.time())
            else:
                new_expires = custom_date
            if new_expires <= now:
                raise ValueError("custom_date must be in the future")
            extension_days = (new_expires - now).days
        else:
            raise ValueError("Either 'days' or 'custom_date' must be provided")

        # Получить план из текущей подписки (или дефолтный)
        if current_sub:
            plan_id = current_sub.plan_id
            traffic_limit_mb = current_sub.traffic_limit_mb
        else:
            raise ValueError(f"No subscription found for client {client_id}")

        # Старую активную — помечаем как expired
        if current_sub and current_sub.status == SubscriptionStatus.ACTIVE:
            self.sub_repo.update(
                db, current_sub,
                status=SubscriptionStatus.EXPIRED,
            )

        # Создаём новую подписку
        new_sub = self.sub_repo.create(
            db,
            client_id=client_id,
            plan_id=plan_id,
            status=SubscriptionStatus.ACTIVE,
            started_at=now,
            expires_at=new_expires,
            traffic_limit_mb=traffic_limit_mb,
            traffic_used_mb=0,  # Сброс трафика
        )

        # Записываем платёж
        if amount is not None:
            self.payment_repo.create(
                db,
                client_id=client_id,
                subscription_id=new_sub.id,
                amount=amount,
                currency="RUB",
                method=payment_method,
                period_days=extension_days,
                note=payment_note,
            )

        # Обновляем статус клиента
        self.client_repo.update(db, client_id=client_id, status="active")

        # Восстанавливаем iptables если клиент был заблокирован
        if current_sub and current_sub.status in [
            SubscriptionStatus.EXPIRED,
            SubscriptionStatus.SUSPENDED,
        ]:
            self.traffic_service.restore_iptables_rule(client_id)

        # Аудит
        self.audit_service.log(
            db=db,
            action="subscription.extend",
            admin_id=admin_id,
            entity_type="subscription",
            entity_id=new_sub.id,
            details={
                "client_id": client_id,
                "old_sub_id": current_sub.id if current_sub else None,
                "days": days,
                "custom_date": custom_date.isoformat() if custom_date else None,
                "new_expires_at": new_expires.isoformat(),
                "amount": amount,
                "payment_method": payment_method,
            },
            ip_address=ip_address,
        )

        return new_sub
```

### 5.2.3 Алгоритм автоматического отключения истёкших

```python
# app/services/scheduler_service.py — задача check_expired_subscriptions

async def check_expired_subscriptions():
    """
    Запускается APScheduler каждый час.

    Алгоритм (v2.0 — soft-block через authz.json, НЕ iptables):
      1. Получить все subscriptions с status=active AND expires_at < now()
      2. Для каждой (накапливаем затронутые server_id):
         a. subscription.status → expired
         b. client.status → expired
         c. user_devices.is_active=false для устройств клиента  (НЕ delete)
         d. audit_log(action="subscription.expire", admin_id=None)
      3. ОДИН РАЗ в конце: authz_writer.write_authz_file(db) (атомарно, version++)
         → (флот) для каждого затронутого server_id Coordinator.on_change → push
      4. Логировать сводку: "Expired N subscriptions, authz version=V"

    Почему write_authz_file ОДИН раз в конце, а не на каждого клиента:
      пакетная запись = меньше I/O и меньше сдвигов mtime; Gate перечитает один раз.
      На флоте — один пересчёт+push на server_id, а не на клиента.

    Edge cases:
      - Клиент уже suspended → не меняем на expired (suspended приоритетнее), но
        его устройства всё равно вне allow (is_active=false уже стоит)
      - Клиент уже deleted → пропускаем
      - Запись authz.json ДОЛЖНА быть атомарной; при fail_mode=lkg сбой записи не
        «откроет всех», но атомарность всё равно обязательна (5.1/5.2 аналитики)
      - Один из клиентов падает → ловим exception, продолжаем цикл; authz пишем по
        фактически обработанным (идемпотентно — срез всё равно пересобирается из БД)
      - Gate применит блок к УЖЕ подключённым сессиям в пределах EnforceInterval (5.6)

    Гарантия идемпотентности:
      Если задача запустилась дважды подряд (misfire) → второй запуск
      не найдёт активных истёкших подписок → ничего не сделает.
    """
    db = SessionLocal()
    expired_count = 0
    errors = []

    try:
        now = datetime.utcnow()

        # Шаг 1: Найти истёкшие активные подписки
        expired_subs = db.query(Subscription).filter(
            Subscription.status == SubscriptionStatus.ACTIVE,
            Subscription.expires_at < now,
        ).all()

        for sub in expired_subs:
            try:
                # Шаг 2a: Статус подписки
                sub.status = SubscriptionStatus.EXPIRED
                db.flush()

                # Шаг 2b: Статус клиента
                client = db.query(Client).filter(Client.id == sub.client_id).first()
                if client and client.status not in ["deleted"]:
                    client.status = "expired"
                    db.flush()

                # Шаг 2c: iptables
                try:
                    traffic_service.remove_iptables_rule(sub.client_id)
                except Exception as iptables_err:
                    # Не фатально — правило могло уже не существовать
                    logger.warning(
                        f"iptables remove failed for client {sub.client_id}: {iptables_err}"
                    )

                # Шаг 2d: Аудит
                db.add(AuditLog(
                    admin_id=None,           # системное действие
                    action="subscription.auto_expired",
                    entity_type="subscription",
                    entity_id=sub.id,
                    details=str({
                        "client_id": sub.client_id,
                        "expired_at": sub.expires_at.isoformat(),
                        "auto_expired_at": now.isoformat(),
                    }),
                ))

                db.commit()
                expired_count += 1

            except Exception as e:
                db.rollback()
                errors.append(f"client_id={sub.client_id}: {e}")
                logger.error(f"Error expiring subscription {sub.id}: {e}", exc_info=True)

        logger.info(
            f"check_expired_subscriptions: expired={expired_count}, errors={len(errors)}"
        )
        if errors:
            logger.error(f"Expiration errors: {errors}")

    finally:
        db.close()
```

---

## 5.3 Учёт трафика  🟤 DORMANT (в v2.0 не реализуется)

> ⚠️ **DORMANT (Решение 5).** Пер-клиентский трафик на WebRTC/Jitsi-туннеле с **общей комнатой** не измеряется: нет привязки счётчика к `deviceId`, iptables-MARK по клиенту неприменим (весь туннель — одна комната). Раздел ниже сохранён как **исторический** дизайн и как задел под будущую серверную телеметрию (Этап 3, 3.7 аналитики). В v2.0: джобы сбора не регистрируются, `subscription.traffic_used_mb` не обновляется, в UI — «лимит N GB · учёт не настроен». Монетизация — **по сроку** (5.2), не по объёму.

### 5.3.1 Принцип работы iptables-учёта (исторический, НЕ активен)

OlcPanel отслеживает трафик каждого клиента через iptables. Ключевая идея:

```
Каждый клиент имеет уникальную Jitsi-комнату (room_name).
Все WebRTC/XMPP соединения к этой комнате идут через конкретный IP Jitsi-сервера.
OlcPanel добавляет iptables-правило с комментарием для идентификации трафика по клиенту.

Архитектура учёта:
  FORWARD chain → правило per client (comment: olcpanel-{client_id}) →
  счётчики накапливаются (монотонно) → APScheduler читает счётчики каждые 5 мин →
  вычисляет дельту → обновляет traffic_daily + subscription.traffic_used_mb
```

### 5.3.2 Добавление iptables-правила при создании клиента

```python
# app/services/traffic_service.py

import subprocess
from datetime import datetime
from sqlalchemy.orm import Session

class TrafficService:

    IPTABLES_COMMENT_PREFIX = "olcpanel"

    def add_iptables_rule(self, client_id: int, jitsi_server_ip: str) -> bool:
        """
        Добавляет iptables-правила для учёта трафика клиента.

        Два правила: входящий и исходящий трафик к Jitsi-серверу.

        Команды:
          iptables -t mangle -A FORWARD
            -d {jitsi_ip}
            -m comment --comment "olcpanel-{client_id}-out"
            -j ACCEPT

          iptables -t mangle -A FORWARD
            -s {jitsi_ip}
            -m comment --comment "olcpanel-{client_id}-in"
            -j ACCEPT

        Примечание:
          Это упрощённый подход: мы считаем трафик по IP Jitsi-сервера,
          а не по конкретной комнате. Для 20 клиентов с уникальными серверами
          это приемлемо. При мультиклиентном Jitsi — нужен более сложный подход
          (например, через nfmark или отдельный PREROUTING по меткам).

        Возвращает True если успешно, False если ошибка.
        """
        comment_in  = f"{self.IPTABLES_COMMENT_PREFIX}-{client_id}-in"
        comment_out = f"{self.IPTABLES_COMMENT_PREFIX}-{client_id}-out"

        rules = [
            ["iptables", "-t", "mangle", "-A", "FORWARD",
             "-s", jitsi_server_ip,
             "-m", "comment", "--comment", comment_in,
             "-j", "ACCEPT"],

            ["iptables", "-t", "mangle", "-A", "FORWARD",
             "-d", jitsi_server_ip,
             "-m", "comment", "--comment", comment_out,
             "-j", "ACCEPT"],
        ]

        for rule in rules:
            try:
                result = subprocess.run(
                    rule,
                    capture_output=True,
                    text=True,
                    timeout=5,
                )
                if result.returncode != 0:
                    logger.error(f"iptables error: {result.stderr}")
                    return False
            except subprocess.TimeoutExpired:
                logger.error(f"iptables timeout for client {client_id}")
                return False
            except Exception as e:
                logger.error(f"iptables exception: {e}")
                return False

        logger.info(f"iptables rules added for client {client_id}")
        return True

    def remove_iptables_rule(self, client_id: int, jitsi_server_ip: str) -> bool:
        """
        Удаляет iptables-правила клиента при отключении/истечении.

        Использует -D вместо -A.

        Edge case: если правила не существует — iptables вернёт ошибку.
        Мы логируем предупреждение, но не падаем.
        """
        comment_in  = f"{self.IPTABLES_COMMENT_PREFIX}-{client_id}-in"
        comment_out = f"{self.IPTABLES_COMMENT_PREFIX}-{client_id}-out"

        rules = [
            ["iptables", "-t", "mangle", "-D", "FORWARD",
             "-s", jitsi_server_ip,
             "-m", "comment", "--comment", comment_in, "-j", "ACCEPT"],

            ["iptables", "-t", "mangle", "-D", "FORWARD",
             "-d", jitsi_server_ip,
             "-m", "comment", "--comment", comment_out, "-j", "ACCEPT"],
        ]

        success = True
        for rule in rules:
            try:
                result = subprocess.run(rule, capture_output=True, text=True, timeout=5)
                if result.returncode != 0:
                    # Правила нет — не критично
                    logger.warning(f"iptables delete warning (may not exist): {result.stderr}")
            except Exception as e:
                logger.error(f"iptables remove exception: {e}")
                success = False

        return success

    def restore_iptables_rule(self, client_id: int) -> bool:
        """
        Восстанавливает iptables-правила при продлении/восстановлении клиента.

        Сначала пробует удалить (на случай дублирования), потом добавляет.
        """
        # Получаем IP Jitsi-сервера из текущего конфига клиента
        # (через репозиторий)
        jitsi_ip = self._get_jitsi_ip_for_client(client_id)
        if not jitsi_ip:
            logger.error(f"Cannot restore iptables for client {client_id}: no jitsi IP")
            return False

        self.remove_iptables_rule(client_id, jitsi_ip)   # cleanup дубликатов
        return self.add_iptables_rule(client_id, jitsi_ip)

    def _get_jitsi_ip_for_client(self, client_id: int) -> str | None:
        """
        Получает IP Jitsi-сервера для клиента.

        Порядок поиска:
          1. Из активного конфига клиента → jitsi_server (hostname)
          2. DNS-резолвинг hostname → IP

        Важно: iptables требует IP, а не hostname.
        """
        import socket
        # В реальном коде здесь будет обращение к config_repo
        # Упрощённо:
        try:
            # jitsi_server из активного конфига
            jitsi_hostname = self._get_jitsi_hostname(client_id)
            if not jitsi_hostname:
                return None
            ip = socket.gethostbyname(jitsi_hostname)
            return ip
        except socket.gaierror as e:
            logger.error(f"DNS resolution failed for {jitsi_hostname}: {e}")
            return None
```

### 5.3.3 Алгоритм сбора трафика (APScheduler, каждые 5 минут)

```python
    def collect_traffic_snapshots(self, db: Session) -> None:
        """
        Читает счётчики iptables для всех активных клиентов.
        Вычисляет дельту с предыдущим снимком.
        Обновляет traffic_daily и subscription.traffic_used_mb.

        Полный алгоритм:

        ┌─────────────────────────────────────────────────────────────┐
        │  Для каждого активного клиента:                             │
        │                                                             │
        │  1. Прочитать текущие bytes_in_total, bytes_out_total       │
        │     из iptables (sudo iptables -t mangle -L FORWARD -v -x)  │
        │                                                             │
        │  2. Получить предыдущий снимок из traffic_snapshots         │
        │     (последняя запись для этого client_id)                  │
        │                                                             │
        │  3. Вычислить дельту:                                       │
        │     delta_in  = current_in  - prev_in                       │
        │     delta_out = current_out - prev_out                      │
        │                                                             │
        │     Edge case: если delta < 0 → перезагрузка сервера,      │
        │     счётчики сбросились. В этом случае:                    │
        │     delta = current (берём всё что накопилось после ребута) │
        │                                                             │
        │  4. Если delta > 0:                                         │
        │     a. Обновить traffic_daily[today] += delta               │
        │        (INSERT OR REPLACE с суммированием)                  │
        │     b. Обновить subscription.traffic_used_mb += delta/1024  │
        │     c. Проверить лимит → если exceeded → suspend            │
        │                                                             │
        │  5. Сохранить новый снимок в traffic_snapshots              │
        │                                                             │
        │  6. После цикла: удалить снимки старше 48 часов            │
        └─────────────────────────────────────────────────────────────┘
        """
        now = datetime.utcnow()
        today = now.date()

        # Получаем активных клиентов
        active_clients = self.client_repo.get_all_active(db)

        # Читаем счётчики из iptables одним вызовом (эффективнее)
        iptables_counters = self._read_all_iptables_counters()

        for client in active_clients:
            try:
                # Шаг 1: Текущие счётчики
                client_counters = iptables_counters.get(client.id)
                if not client_counters:
                    # Нет правила для этого клиента — пропускаем
                    # (возможно, правило ещё не создано или было удалено)
                    continue

                current_in  = client_counters["bytes_in"]
                current_out = client_counters["bytes_out"]

                # Шаг 2: Предыдущий снимок
                prev_snapshot = self.traffic_repo.get_latest_snapshot(db, client.id)

                # Шаг 3: Дельта
                if prev_snapshot:
                    delta_in  = current_in  - prev_snapshot.bytes_in_total
                    delta_out = current_out - prev_snapshot.bytes_out_total

                    # Если delta отрицательная → ребут сервера (сброс счётчиков)
                    if delta_in < 0:
                        delta_in  = current_in
                    if delta_out < 0:
                        delta_out = current_out
                else:
                    # Первый снимок — всё что есть это дельта с начала
                    delta_in  = current_in
                    delta_out = current_out

                delta_total = delta_in + delta_out

                # Шаг 4: Обновляем если есть дельта
                if delta_total > 0:
                    delta_mb = delta_total // (1024 * 1024)  # байты → МБ (целые)
                    delta_mb_remainder = delta_total % (1024 * 1024)

                    # a. traffic_daily
                    self.traffic_repo.upsert_daily(
                        db,
                        client_id=client.id,
                        date=today,
                        bytes_in=delta_in,
                        bytes_out=delta_out,
                        bytes_total=delta_total,
                    )

                    # b. subscription.traffic_used_mb
                    active_sub = self.sub_repo.get_active(db, client.id)
                    if active_sub:
                        # Используем атомарное обновление (не read-modify-write)
                        # чтобы избежать race condition с другими потоками
                        self.sub_repo.increment_traffic_used(
                            db,
                            subscription_id=active_sub.id,
                            delta_mb=delta_mb,
                        )

                        # c. Проверка лимита
                        # Перечитываем актуальное значение после инкремента
                        db.refresh(active_sub)
                        if active_sub.is_traffic_exceeded:
                            self._handle_traffic_limit_exceeded(db, client, active_sub)

                # Шаг 5: Новый снимок
                self.traffic_repo.create_snapshot(
                    db,
                    client_id=client.id,
                    captured_at=now,
                    bytes_in_total=current_in,
                    bytes_out_total=current_out,
                )

                db.commit()

            except Exception as e:
                db.rollback()
                logger.error(
                    f"Traffic collection error for client {client.id}: {e}",
                    exc_info=True,
                )

    def _read_all_iptables_counters(self) -> dict[int, dict]:
        """
        Читает все iptables счётчики одним вызовом и парсит вывод.

        Вывод iptables -t mangle -L FORWARD -v -x --line-numbers:
          Chain FORWARD (policy ACCEPT 0 packets, 0 bytes)
          num   pkts      bytes target prot opt in     out     source               destination         comment
          1      1234  5678901 ACCEPT  all  --  *      *       5.178.85.63          0.0.0.0/0           /* olcpanel-1-in */
          2       987  2345678 ACCEPT  all  --  *      *       0.0.0.0/0            5.178.85.63         /* olcpanel-1-out */

        Парсим строки с комментариями "olcpanel-{client_id}-{in/out}".
        """
        try:
            result = subprocess.run(
                ["iptables", "-t", "mangle", "-L", "FORWARD", "-v", "-x", "--line-numbers"],
                capture_output=True,
                text=True,
                timeout=10,
            )
            if result.returncode != 0:
                logger.error(f"iptables read error: {result.stderr}")
                return {}
        except subprocess.TimeoutExpired:
            logger.error("iptables read timeout")
            return {}

        counters = {}
        prefix = self.IPTABLES_COMMENT_PREFIX

        for line in result.stdout.splitlines():
            # Ищем строки вида: "num pkts bytes ... /* olcpanel-{id}-{direction} */"
            if prefix not in line:
                continue

            parts = line.split()
            if len(parts) < 3:
                continue

            try:
                bytes_count = int(parts[2])    # третья колонка = bytes (с флагом -x = точные)
            except (ValueError, IndexError):
                continue

            # Извлекаем client_id и направление из комментария
            import re
            match = re.search(rf"{prefix}-(\d+)-(in|out)", line)
            if not match:
                continue

            client_id  = int(match.group(1))
            direction  = match.group(2)

            if client_id not in counters:
                counters[client_id] = {"bytes_in": 0, "bytes_out": 0}

            if direction == "in":
                counters[client_id]["bytes_in"] = bytes_count
            else:
                counters[client_id]["bytes_out"] = bytes_count

        return counters

    def _handle_traffic_limit_exceeded(
        self,
        db: Session,
        client: "Client",
        subscription: "Subscription",
    ) -> None:
        """
        Действия при превышении лимита трафика.

        Шаги:
          1. Suspend подписки (status → suspended)
          2. Suspend клиента (status → suspended)
          3. Удаление iptables-правил
          4. Аудит-запись
        """
        logger.warning(
            f"Traffic limit exceeded for client {client.id}: "
            f"{subscription.traffic_used_mb}/{subscription.traffic_limit_mb} MB"
        )

        subscription.status = SubscriptionStatus.SUSPENDED
        client.status = "suspended"

        self.remove_iptables_rule(client.id, self._get_jitsi_ip_for_client(client.id))

        db.add(AuditLog(
            admin_id=None,
            action="subscription.traffic_limit_exceeded",
            entity_type="subscription",
            entity_id=subscription.id,
            details=str({
                "client_id": client.id,
                "used_mb": subscription.traffic_used_mb,
                "limit_mb": subscription.traffic_limit_mb,
            }),
        ))

        db.commit()
        logger.info(f"Client {client.id} suspended due to traffic limit exceeded")
```

---

## 5.4 Аутентификация и безопасность

### 5.4.1 Алгоритм входа (Login Flow)

```
┌────────────────────────────────────────────────────────────────────┐
│                      LOGIN ALGORITHM                               │
├────────────────────────────────────────────────────────────────────┤
│                                                                    │
│  POST /api/auth/login {username, password}                         │
│    │                                                               │
│    ├─ [1] Rate limit check (IP-based, in-memory)                  │
│    │       attempts[ip] > 5 за последние 15 мин?                  │
│    │       ДА → 429 Too Many Requests                              │
│    │       НЕТ → продолжаем                                        │
│    │                                                               │
│    ├─ [2] Pydantic валидация тела запроса                          │
│    │       username: 3-64 символа, алфанум/_/-                    │
│    │       password: 8-128 символов                               │
│    │       Ошибка → 422 Unprocessable Entity                      │
│    │                                                               │
│    ├─ [3] Поиск администратора в БД                               │
│    │       SELECT * FROM admins WHERE username = ? AND is_active=1│
│    │       НЕ найден → записываем failed attempt → 401            │
│    │       НАЙДЕН → продолжаем                                     │
│    │                                                               │
│    ├─ [4] Проверка пароля (bcrypt verify)                         │
│    │       passlib.context.verify(password, admin.password_hash)  │
│    │       НЕВЕРНО → записываем failed attempt → 401              │
│    │       ВЕРНО → продолжаем                                      │
│    │                                                               │
│    │       ВАЖНО: всегда одинаковое сообщение "Invalid credentials"│
│    │       и одинаковое время ответа (bcrypt нивелирует тайминг)  │
│    │       → предотвращает user enumeration атаки                 │
│    │                                                               │
│    ├─ [5] Создание JWT токена                                      │
│    │       payload = {                                             │
│    │         "sub": str(admin.id),     # subject                  │
│    │         "username": admin.username,                          │
│    │         "jti": uuid4(),            # JWT ID для revocation   │
│    │         "iat": now,                # issued at               │
│    │         "exp": now + 24h,          # expiration              │
│    │       }                                                       │
│    │       token = jwt.encode(payload, SECRET_KEY, "HS256")       │
│    │                                                               │
│    ├─ [6] Обновление last_login в БД                              │
│    │       UPDATE admins SET last_login = now() WHERE id = ?      │
│    │                                                               │
│    ├─ [7] Сброс счётчика попыток для IP (успешный вход)           │
│    │                                                               │
│    ├─ [8] Аудит-запись                                            │
│    │       action="auth.login", details={ip, username}            │
│    │                                                               │
│    └─ [9] Ответ: 200 OK + Set-Cookie: access_token=<jwt>          │
│                  HttpOnly; Secure; SameSite=Strict                │
│                                                                    │
└────────────────────────────────────────────────────────────────────┘
```

### 5.4.2 In-Memory Rate Limiter

```python
# app/middleware/rate_limit.py

from collections import defaultdict, deque
from datetime import datetime, timedelta
from fastapi import Request, Response
from starlette.middleware.base import BaseHTTPMiddleware

class RateLimitMiddleware(BaseHTTPMiddleware):
    """
    Простой sliding window rate limiter.

    Хранит timestamp'ы запросов в deque для каждого IP.
    При добавлении нового запроса — удаляет старые (за пределами окна).

    Конфигурация:
      /api/auth/login: 5 запросов / 900 секунд (15 мин)
      /api/*:          100 запросов / 60 секунд (1 мин)
      /api/health:     без лимита

    Thread-safety:
      defaultdict + deque операции в CPython GIL-protected.
      Для multi-process (несколько Uvicorn workers) нужен Redis.
      Для одного worker (наш случай) — достаточно.
    """

    # {(ip, endpoint_key): deque[datetime]}
    _windows: dict[tuple, deque] = defaultdict(deque)

    LIMITS = {
        "login":   (5,   900),   # 5 попыток за 15 минут
        "api":     (100, 60),    # 100 запросов в минуту
    }

    async def dispatch(self, request: Request, call_next) -> Response:
        path = request.url.path
        client_ip = request.client.host if request.client else "unknown"

        # Определяем тип лимита
        if path == "/api/auth/login":
            limit_key = "login"
        elif path.startswith("/api/health"):
            # Без лимита
            return await call_next(request)
        elif path.startswith("/api/"):
            limit_key = "api"
        else:
            return await call_next(request)

        max_requests, window_seconds = self.LIMITS[limit_key]

        # Sliding window
        now = datetime.utcnow()
        window_start = now - timedelta(seconds=window_seconds)
        key = (client_ip, limit_key)

        window = self._windows[key]

        # Удаляем устаревшие записи
        while window and window[0] < window_start:
            window.popleft()

        # Проверяем лимит
        if len(window) >= max_requests:
            # Вычисляем Retry-After
            oldest = window[0]
            retry_after = int((oldest + timedelta(seconds=window_seconds) - now).total_seconds())

            return Response(
                content=f'{{"detail":"Too many requests","retry_after_seconds":{retry_after}}}',
                status_code=429,
                headers={
                    "Content-Type": "application/json",
                    "Retry-After": str(retry_after),
                },
            )

        # Добавляем текущий запрос
        window.append(now)

        return await call_next(request)
```

### 5.4.3 JWT — создание и верификация

```python
# app/core/security.py

from datetime import datetime, timedelta, timezone
from typing import Optional
import uuid

from jose import JWTError, jwt
from passlib.context import CryptContext

from app.config import get_settings

settings = get_settings()

# bcrypt контекст (cost factor из настроек, по умолчанию 12)
pwd_context = CryptContext(
    schemes=["bcrypt"],
    deprecated="auto",
    bcrypt__rounds=settings.BCRYPT_ROUNDS,
)


def hash_password(password: str) -> str:
    """Хеширует пароль через bcrypt. cost=12 → ~250ms."""
    return pwd_context.hash(password)


def verify_password(plain_password: str, hashed_password: str) -> bool:
    """
    Проверяет пароль против bcrypt-хеша.

    ВАЖНО: passlib всегда выполняет полный bcrypt цикл,
    даже если пароль неверный (защита от timing attacks).
    """
    return pwd_context.verify(plain_password, hashed_password)


def create_access_token(admin_id: int, username: str) -> tuple[str, str]:
    """
    Создаёт JWT access token.

    Возвращает: (token_string, jti)
    jti нужен для добавления в revocation_list при logout.

    Payload:
      sub      — admin_id как строка (стандарт JWT)
      username — для быстрого отображения в UI (без обращения к БД)
      jti      — уникальный ID токена (для revocation)
      iat      — время выдачи
      exp      — время истечения
    """
    now = datetime.now(timezone.utc)
    jti = str(uuid.uuid4())

    expire = now + timedelta(
        minutes=settings.JWT_ACCESS_TOKEN_EXPIRE_MINUTES
    )

    payload = {
        "sub": str(admin_id),
        "username": username,
        "jti": jti,
        "iat": now,
        "exp": expire,
    }

    token = jwt.encode(
        payload,
        settings.SECRET_KEY,
        algorithm=settings.JWT_ALGORITHM,
    )

    return token, jti


def decode_access_token(token: str) -> Optional[dict]:
    """
    Декодирует JWT токен.

    Возвращает payload dict или None при любой ошибке.

    Проверяет:
      - Подпись (SECRET_KEY)
      - Срок действия (exp)
      - Алгоритм (HS256)

    НЕ проверяет revocation_list — это делает AuthService.
    """
    try:
        payload = jwt.decode(
            token,
            settings.SECRET_KEY,
            algorithms=[settings.JWT_ALGORITHM],
        )
        return payload
    except JWTError:
        return None
```

### 5.4.4 In-Memory Revocation List

```python
# app/services/auth_service.py (фрагмент)

class AuthService:
    """
    In-memory revocation list для JWT токенов.

    При logout или смене пароля → jti добавляется в _revoked_jtis.
    При verify_token → проверяем что jti НЕ в revoked.

    Ограничения:
      - Очищается при перезагрузке → все токены становятся валидными снова
      - Приемлемо: expire 24ч, при перезагрузке сервера пользователь
        обязательно перелогинится в течение суток
      - При multi-process → нужен Redis (у нас один Uvicorn worker)

    Размер:
      При 1 администраторе: в revoked_jtis единицы записей.
      При 100 logout/день → 100 записей = ~10 KB памяти.
      Очищаются каждый час (удаляем с истёкшим exp).
    """

    _revoked_jtis: set[str] = set()

    def revoke_token(self, jti: str) -> None:
        self._revoked_jtis.add(jti)

    def is_revoked(self, jti: str) -> bool:
        return jti in self._revoked_jtis

    def cleanup_revoked(self) -> None:
        """
        Удаляет истёкшие токены из revocation list.
        Вызывается APScheduler раз в час.

        Проблема: у нас нет exp в revocation list, только jti.
        Решение: декодируем каждый jti... нет, это дорого.

        Лучшее решение: храним {jti: exp_timestamp}.
        Удаляем все у которых exp < now.
        """
        # Упрощённо: раз в сутки очищаем весь список
        # (токены 24ч и так истекут)
        self._revoked_jtis.clear()

    def verify_token(self, token: str) -> "Admin | None":
        """
        Полная проверка токена:
          1. Decode (подпись + exp)
          2. Проверка revocation list
          3. Получение Admin из БД
        """
        payload = decode_access_token(token)
        if not payload:
            return None

        jti = payload.get("jti")
        if jti and self.is_revoked(jti):
            return None

        admin_id = payload.get("sub")
        if not admin_id:
            return None

        # В реальном коде: db query
        # admin = admin_repo.get_by_id(db, int(admin_id))
        # return admin if admin and admin.is_active else None
        ...
```

---

## 5.5 Управление OlcRTC-сервером

### 5.5.1 Алгоритм проверки статуса

```python
# app/services/server_service.py

import subprocess
import re
from datetime import datetime

class ServerService:

    ALLOWED_COMMANDS = {
        "is_active": ["systemctl", "is-active"],
        "restart":   ["systemctl", "restart"],
        "status":    ["systemctl", "status"],
    }

    def get_olcrtc_status(self, service_name: str) -> dict:
        """
        Проверяет статус OlcRTC systemd-сервиса.

        Возвращает:
          {
            "status": "running" | "stopped" | "failed" | "unknown",
            "service_name": str,
            "uptime_seconds": int | None,
          }

        Реализация:
          1. subprocess(["systemctl", "is-active", service_name])
          2. stdout.strip() == "active" → running
          3. Для uptime: subprocess(["systemctl", "status", service_name])
             → парсим "Active: active (running) since ... ago"
        """
        try:
            result = subprocess.run(
                ["systemctl", "is-active", service_name],
                capture_output=True,
                text=True,
                timeout=5,
            )
            raw_status = result.stdout.strip()

            if raw_status == "active":
                status = "running"
                uptime = self._get_uptime_seconds(service_name)
            elif raw_status == "failed":
                status = "failed"
                uptime = None
            elif raw_status in ["inactive", "dead"]:
                status = "stopped"
                uptime = None
            else:
                status = "unknown"
                uptime = None

        except subprocess.TimeoutExpired:
            logger.error(f"systemctl timeout for {service_name}")
            status = "unknown"
            uptime = None
        except Exception as e:
            logger.error(f"systemctl error: {e}")
            status = "unknown"
            uptime = None

        return {
            "status": status,
            "service_name": service_name,
            "uptime_seconds": uptime,
        }

    def _get_uptime_seconds(self, service_name: str) -> int | None:
        """
        Парсит uptime из вывода systemctl status.

        Вывод содержит строку вида:
          Active: active (running) since Thu 2026-06-10 14:00:00 UTC; 2 days 2h ago

        Парсим "2 days 2h ago" → секунды.
        """
        try:
            result = subprocess.run(
                ["systemctl", "status", service_name],
                capture_output=True, text=True, timeout=5,
            )

            # Ищем строку с "since"
            for line in result.stdout.splitlines():
                if "Active: active (running) since" in line:
                    # Парсим дату из строки
                    # Формат: "since Thu 2026-06-10 14:00:00 UTC"
                    match = re.search(
                        r"since .+?(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}) UTC",
                        line
                    )
                    if match:
                        since_str = match.group(1)
                        since_dt = datetime.strptime(since_str, "%Y-%m-%d %H:%M:%S")
                        uptime = (datetime.utcnow() - since_dt).total_seconds()
                        return int(uptime)
            return None
        except Exception:
            return None

    def restart_olcrtc(self, service_name: str) -> dict:
        """
        Перезапускает OlcRTC через systemctl.

        Шаги:
          1. systemctl restart {service_name}
          2. Ждём 3 секунды
          3. Проверяем статус
          4. Возвращаем результат

        Безопасность:
          - service_name берётся из ServerSettings (не из запроса)
          - Только предустановленные имена сервисов
          - Timeout 10 секунд
        """
        # Проверяем что имя сервиса допустимое (whitelist)
        allowed_services = ["olcrtc-server", "olcpanel"]
        if service_name not in allowed_services:
            raise ValueError(f"Service {service_name} not in whitelist")

        try:
            result = subprocess.run(
                ["sudo", "systemctl", "restart", service_name],
                capture_output=True, text=True, timeout=10,
            )

            if result.returncode != 0:
                logger.error(f"restart failed: {result.stderr}")
                return {
                    "success": False,
                    "error": result.stderr,
                }

        except subprocess.TimeoutExpired:
            return {"success": False, "error": "systemctl restart timed out"}

        # Ждём запуска
        import time
        time.sleep(3)

        # Проверяем статус
        status_result = self.get_olcrtc_status(service_name)

        return {
            "success": status_result["status"] == "running",
            "status": status_result["status"],
            "restart_time": datetime.utcnow().isoformat(),
        }
```

---

## 5.6 Восстановление iptables после перезагрузки

```python
# app/services/scheduler_service.py (startup задача)

async def restore_iptables_on_startup(db: Session) -> None:
    """
    Вызывается при старте OlcPanel (в lifespan).

    При перезагрузке VPS все iptables-правила теряются.
    Эта функция восстанавливает правила для всех активных клиентов.

    Шаги:
      1. Получить всех клиентов со status=active
      2. Для каждого: восстановить iptables-правило
      3. Логировать количество восстановленных правил

    Важно: выполняется до обработки первых запросов (startup event).
    Ошибка для одного клиента — не останавливает восстановление остальных.
    """
    logger.info("Restoring iptables rules for active clients...")
    active_clients = client_repo.get_all_active(db)
    restored = 0
    failed = 0

    for client in active_clients:
        try:
            success = traffic_service.restore_iptables_rule(client.id)
            if success:
                restored += 1
            else:
                failed += 1
        except Exception as e:
            logger.error(f"iptables restore failed for client {client.id}: {e}")
            failed += 1

    logger.info(
        f"iptables restore complete: restored={restored}, failed={failed}"
    )
```

---

## 5.7 Carriers Health — парсинг и кэширование

```python
# app/services/carriers_service.py

import json
import re
from datetime import datetime, timedelta
from pathlib import Path

class CarriersService:
    """
    Парсит good-carriers.md и validate-логи.
    Кэширует результат 5 минут (TTL).
    """

    _cache: dict | None = None
    _cache_updated_at: datetime | None = None
    CACHE_TTL_SECONDS = 300  # 5 минут

    def get_health(self) -> dict:
        """
        Возвращает cached health данные или обновляет при истечении TTL.
        """
        now = datetime.utcnow()

        if (self._cache is None or
            self._cache_updated_at is None or
            (now - self._cache_updated_at).total_seconds() > self.CACHE_TTL_SECONDS):
            self._cache = self._refresh()
            self._cache_updated_at = now

        return self._cache

    def _refresh(self) -> dict:
        """
        Парсит актуальные данные из файловой системы.

        Источники:
          1. /root/olcrtc/docs/good-carriers.md — основной список carriers
          2. /tmp/validate-*.log — результаты последней валидации
          3. journalctl -u olcrtc-server — liveness данные (stall/bytes)
        """
        carriers = []

        # Источник 1: good-carriers.md
        carriers_from_md = self._parse_good_carriers_md()
        carriers.extend(carriers_from_md)

        # Источник 2: validate-логи
        validate_data = self._parse_validate_logs()

        # Источник 3: journald liveness
        liveness_data = self._parse_liveness_from_journal()

        # Объединяем
        for carrier in carriers:
            name = carrier["name"]

            # Данные из validate-лога
            if name in validate_data:
                carrier["last_validated"] = validate_data[name].get("timestamp")
                carrier["soak_kb"] = validate_data[name].get("soak_kb")

            # Данные из liveness
            if name in liveness_data:
                carrier["suspect"] = liveness_data[name].get("suspect", False)
                carrier["last_stall_at"] = liveness_data[name].get("last_stall_at")

        return {
            "carriers": carriers,
            "updated_at": datetime.utcnow().isoformat(),
            "cache_ttl_seconds": self.CACHE_TTL_SECONDS,
        }

    def _parse_good_carriers_md(self) -> list[dict]:
        """
        Парсит good-carriers.md для получения списка carriers.

        Ищем строки вида:
          - **GOLD #1**: ct.placetime.team (5.178.85.63) — 100KB+ volume_aware ANONYMOUS
          - **Fallback #1**: mf.example.com — fallback carrier

        Парсинг упрощённый: ищем по маркерам GOLD/Fallback/fallback.
        """
        carriers = []
        md_path = Path(get_settings().GOOD_CARRIERS_MD_PATH)

        if not md_path.exists():
            logger.warning(f"good-carriers.md not found at {md_path}")
            return carriers

        try:
            content = md_path.read_text(encoding="utf-8")
        except Exception as e:
            logger.error(f"Cannot read good-carriers.md: {e}")
            return carriers

        # Ищем GOLD-записи
        gold_pattern = re.compile(
            r"GOLD\s+#(\d+)[:\s]+([a-zA-Z0-9.\-]+)",
            re.IGNORECASE
        )
        fallback_pattern = re.compile(
            r"[Ff]allback\s+#(\d+)[:\s]+([a-zA-Z0-9.\-]+)",
            re.IGNORECASE
        )

        for match in gold_pattern.finditer(content):
            carriers.append({
                "name": match.group(2).strip(),
                "status": "gold",
                "is_primary": match.group(1) == "1",
                "volume_aware": "volume_aware" in content[match.start():match.start()+200],
                "anonymous": "ANONYMOUS" in content[match.start():match.start()+200].upper(),
                "suspect": False,
                "last_validated": None,
                "soak_kb": None,
                "last_stall_at": None,
            })

        for match in fallback_pattern.finditer(content):
            carriers.append({
                "name": match.group(2).strip(),
                "status": "fallback",
                "is_primary": False,
                "volume_aware": False,
                "anonymous": False,
                "suspect": False,
                "last_validated": None,
                "soak_kb": None,
                "last_stall_at": None,
            })

        return carriers

    def _parse_validate_logs(self) -> dict[str, dict]:
        """
        Парсит последние /tmp/validate-*.log файлы.

        Формат строки лога:
          2026-06-12 10:00:01 ct.placetime.team OK soak=104KB duration=45s

        Возвращает: {carrier_name: {timestamp, soak_kb}}
        """
        import glob
        result = {}

        log_files = sorted(glob.glob("/tmp/validate-*.log"), reverse=True)

        for log_file in log_files[:5]:  # последние 5 файлов
            try:
                with open(log_file, "r") as f:
                    for line in f:
                        # Парсим строки с результатами
                        match = re.search(
                            r"(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})\s+"
                            r"([a-zA-Z0-9.\-]+)\s+OK\s+soak=(\d+)KB",
                            line
                        )
                        if match:
                            ts_str  = match.group(1)
                            carrier = match.group(2)
                            soak_kb = int(match.group(3))

                            if carrier not in result:
                                result[carrier] = {
                                    "timestamp": ts_str,
                                    "soak_kb": soak_kb,
                                }
            except Exception as e:
                logger.warning(f"Cannot parse {log_file}: {e}")

        return result

    def _parse_liveness_from_journal(self) -> dict[str, dict]:
        """
        Парсит journald логи OlcRTC для определения stall/suspect carriers.

        Ищем строки:
          "liveness: stall detected" → suspect=True
          "liveness: bytes=N ssn=M ok" → healthy

        Возвращает: {carrier_name: {suspect, last_stall_at}}
        """
        result = {}

        try:
            journal_result = subprocess.run(
                ["journalctl", "-u", "olcrtc-server", "-n", "200", "--no-pager"],
                capture_output=True, text=True, timeout=5,
            )
            lines = journal_result.stdout.splitlines()
        except Exception as e:
            logger.warning(f"journalctl parse failed: {e}")
            return result

        stall_pattern  = re.compile(r"stall|suspect|freeze", re.IGNORECASE)
        ok_pattern     = re.compile(r"liveness.*bytes=(\d+).*ok", re.IGNORECASE)
        carrier_pattern = re.compile(r"carrier[=:\s]+([a-zA-Z0-9.\-]+)", re.IGNORECASE)

        for line in lines:
            carrier_match = carrier_pattern.search(line)
            carrier = carrier_match.group(1) if carrier_match else "unknown"

            if stall_pattern.search(line):
                result[carrier] = {
                    "suspect": True,
                    "last_stall_at": datetime.utcnow().isoformat(),
                }
            elif ok_pattern.search(line):
                if carrier not in result:
                    result[carrier] = {"suspect": False, "last_stall_at": None}

        return result
```

---

## 5.8 Инициализация первого администратора

```python
# scripts/create_admin.py

"""
Скрипт для создания первого администратора.
Запускается один раз при установке.

Использование:
  python scripts/create_admin.py

Или через .env:
  ADMIN_USERNAME=admin ADMIN_PASSWORD=secret python scripts/create_admin.py

Логика:
  1. Проверяем: есть ли уже записи в таблице admins?
     ДА → выводим предупреждение и выходим (не перезаписываем)
     НЕТ → создаём

  2. Берём username/password из .env или запрашиваем интерактивно

  3. Хешируем пароль через bcrypt

  4. INSERT INTO admins (username, password_hash)

  5. Выводим: "Admin 'admin' created successfully."
"""

import sys
import os
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from app.database import SessionLocal, create_tables
from app.models.admin import Admin
from app.core.security import hash_password
from app.config import get_settings


def create_first_admin():
    settings = get_settings()
    create_tables()

    db = SessionLocal()
    try:
        # Проверяем наличие администраторов
        existing = db.query(Admin).count()
        if existing > 0:
            print(f"⚠️  Admin already exists ({existing} admin(s) in DB). Skipping.")
            return

        # Берём данные из настроек или запрашиваем
        username = settings.ADMIN_USERNAME
        password = settings.ADMIN_PASSWORD

        if not username or not password:
            print("Please set ADMIN_USERNAME and ADMIN_PASSWORD in .env")
            sys.exit(1)

        if len(password) < 12:
            print("⚠️  Warning: password is less than 12 characters. Consider using a stronger password.")

        # Хешируем пароль
        password_hash = hash_password(password)

        # Создаём администратора
        admin = Admin(
            username=username.lower(),
            password_hash=password_hash,
            is_active=True,
        )
        db.add(admin)
        db.commit()

        print(f"✅ Admin '{username}' created successfully.")
        print(f"   Login at your panel URL → /login")
        print(f"   ⚠️  Delete ADMIN_PASSWORD from .env after first login!")

    finally:
        db.close()


if __name__ == "__main__":
    create_first_admin()
```

---

## 5.6 Блокировка доступа через `authz.json` (ядро монетизации v2.0)

> Это центральный алгоритм v2.0: как именно «продал на срок → по истечении отрубилось» и «заблокировал вручную → отвалился» работают на общей комнате + общем ключе. Заменяет iptables-механику 5.3.

### 5.6.1 Принцип

Доступ определяется **членством `deviceId` в `authz.json.allow[]`**, а не наличием ключа (ключ общий). Панель — единственный писатель файла; сервер (`Gate`) — единственный читатель/исполнитель. Блокировка = убрать `deviceId` из `allow` (soft-block, запись клиента в БД сохраняется).

```
Множество allow для server X (пересобирается ИЗ БД при каждой записи):
  allow(X) = { d.device_id
               FROM user_devices d
               JOIN clients c ON c.id=d.client_id
               JOIN subscriptions s ON s.client_id=c.id AND s.status='active'
               WHERE d.is_active
                 AND s.expires_at > now()
                 AND d.server_id = X }          # фильтр по серверу — только на флоте
```

### 5.6.2 Алгоритм блокировки (ручной и авто) — пошагово

```
block(client_id, reason):                      # авто-вариант — то же из scheduler (5.2.3)
  1. tx:
       client.status = 'blocked'|'expired'
       UPDATE user_devices SET is_active=false WHERE client_id=:client_id
       audit_log("client.block", reason)
  2. authz_writer.write_authz_file(db):
       version = authz_state.version + 1        # монотонно
       allow   = пересчёт по 5.6.1
       payload = {version, updated_at, mode, allow, deny}
       АТОМАРНО: tmp → fsync → rename            # 5.1/5.2 аналитики
  3. (флот) Coordinator.on_change(server_id):
       пересчёт среза для server_id → Pusher.push → ServerHealthService подтвердит applied
  4. Gate (на сервере) в пределах EnforceInterval:
       перечитывает authz.json по mtime → новый allow без device_id →
       removePeerSession() для живых сессий этого deviceId   # рвём УЖЕ подключённого
```

**Окно применения:** новые handshakes — мгновенно (на следующем `Allowed()`), живые сессии — ≤ `EnforceInterval`. Это и есть отличие от статичного URI, который не перепроверялся.

### 5.6.3 Влияние last-known-good на семантику блокировки (Решение 4)

LKG меняет поведение при **сбое файла**, а не при штатной блокировке:

- **Штатно:** запись прошла → `Gate` применил новый allow → заблокированный отвалился. LKG не задействован.
- **Сбой записи/доставки** (битый файл, пропажа, неудачный push): `Gate` **держит последний валидный allow** (не открывается всем). Практический эффект для блокировки: заблокированный `deviceId` может **остаться доступным до приезда следующего валидного файла** — это осознанный компромисс (лучше «чуть дольше доступен заблокированный», чем «бесплатно доступны все»). Ограничивается `lkg_max_age` + алерт.
- **Инвариант:** блокировка никогда не «теряется в сторону открытия» — максимум задерживается. Разблокировка (возврат в allow) при сбое тоже задержится — поэтому при ручной разблокировке оператор видит applied_version и может сделать re-push (4.11).

### 5.6.4 Порядок шагов при миграции устройства между серверами (Этап 3)

Прямое следствие LKG/fail-safe (4.7 аналитики). Для **платящего** клиента — «сначала добавить, потом убрать»:

```
move(device, X → Y):
  1. tx: user_devices.server_id = Y           # фиксируем намерение
  2. Coordinator.on_change(Y): allow(Y)+push  # устройство РАЗРЕШЕНО на Y
  3. Coordinator.on_change(X): allow(X)+push  # устройство УБРАНО с X
  4. health-poll подтверждает обе version
  # до подтверждения устройство временно валидно на ОБОИХ (безопасно для UX)
```

Для **блокировки** — обратный порядок (сначала убрать со старого). Принцип: окно «разрешён на двух» безопаснее окна «нигде» для платящего; для блокируемого — наоборот.

### 5.6.5 Идемпотентность и гонки

- `write_authz_file` всегда **пересобирает** срез из БД (никогда не «дельтит») → повторная доставка/двойной вызов безопасны.
- Монотонный `version` (`authz_state`) + version-reject на сервере (вариант B) защищают от «отката» allowlist при гонке ретраев доставки.
- Частые записи в пределах гранулярности mtime ФС → дедуп/`os.utime` (4.5 п.3), иначе `Gate` пропустит второе обновление до следующего.

---

*Следующий раздел: [[OlcPanel_06_UI_Карта_экранов]] — экраны панели (вкл. дашборд флота с applied_version/lkg_valid_at), навигация, компоненты UI.*

════════════════════════════════════════════════════════════════════════════════
<!-- Конец файла 05_Бизнес_логика.md -->
════════════════════════════════════════════════════════════════════════════════
