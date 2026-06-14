════════════════════════════════════════════════════════════════════════════════
<!-- ФАЙЛ 7/10: 06_UI_Карта_экранов.md -->
════════════════════════════════════════════════════════════════════════════════

# OlcPanel — Раздел 6: UI — Карта экранов
## Экраны · Wireframes · Навигация · Компоненты · HTML/CSS/JS

> **Версия:** 2.0 | **Статус:** 🟡 В разработке
> **Стек:** Vanilla HTML5 + CSS3 + JavaScript ES2022 + Chart.js (CDN)
>
> ⚠️ **v2.0 UI-сдвиги (детали — раздел 6.10):** (1) на дашборде вместо «Трафик» — **виджет свежести authz/флота** (`applied_version`, возраст `lkg_valid_at`, `load_errors`); «Требуют внимания» включает серверы с устаревшим/битым authz. (2) В карточке клиента вместо «Сменить конфиг»/трафика — **секция «Устройства»** (deviceId, last_used_at, server_id, блок/отвязка) и **ссылка подписки** (QR + ротация). (3) Трафик показывается как «учёт не настроен» (Решение 5). Старые вайрфреймы с пер-клиентским конфигом/трафиком ниже — LEGACY-референс.
> **Философия UI:** «Secure Elegance» — тёмный navy + изумрудные акценты,
>                   минимализм, профессиональный вид, без излишеств
> **Зависимости:** [[04_API]], [[05_Бизнес_логика]]

---

## 6.1 Общая карта навигации

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        КАРТА ЭКРАНОВ OlcPanel                          │
└─────────────────────────────────────────────────────────────────────────┘

  /login                         ← login.html (отдельная страница)
    │
    │ [успешный вход]
    ▼
  /                              ← index.html (SPA, все экраны внутри)
  │
  ├── #dashboard                 ← Дашборд (главная после логина)
  │     виджеты статистики
  │     список "Требуют внимания"
  │     график трафика
  │     статус OlcRTC
  │
  ├── #clients                   ← Список клиентов
  │     поиск + фильтр по статусу
  │     таблица / карточки
  │     кнопки: создать / продлить / конфиг
  │     │
  │     └── #clients/{id}        ← Карточка клиента
  │           основные данные
  │           подписка + трафик (прогресс-бар)
  │           конфиг (URI + QR + кнопка копировать)
  │           история подписок
  │           история платежей
  │           история конфигов
  │
  ├── #carriers                  ← Gold Carriers (статус носителей)
  │     карточки carriers
  │     статус GOLD / fallback / degraded
  │     метрики soak_kb, last_validated
  │     кнопка refresh
  │
  ├── #settings                  ← Настройки
  │     настройки OlcRTC (Jitsi URL)
  │     тарифные планы (CRUD)
  │     логи OlcRTC (последние 100 строк)
  │     кнопка перезапустить OlcRTC
  │     смена пароля
  │
  └── [MODALS — появляются поверх любого экрана]
        ├── Modal: Создать клиента
        ├── Modal: Продлить подписку
        ├── Modal: Отозвать конфиг
        ├── Modal: Приостановить клиента
        ├── Modal: Просмотр QR-кода (крупно)
        └── Modal: Подтверждение опасных действий
```

---

## 6.2 Дизайн-система

### 6.2.1 Цветовая палитра

```css
/* app/static/css/style.css — CSS Variables */

:root {
  /* === Основные цвета === */
  --color-bg-primary:    #0f172a;   /* тёмный navy — основной фон */
  --color-bg-secondary:  #1e293b;   /* чуть светлее — фон карточек, боковая панель */
  --color-bg-tertiary:   #334155;   /* ещё светлее — hover состояния, borders */

  /* === Акцентные цвета === */
  --color-emerald:       #10b981;   /* изумрудный — GOLD статус, активные элементы, кнопки */
  --color-emerald-dark:  #059669;   /* тёмнее изумрудного — hover кнопок */
  --color-emerald-glow:  rgba(16, 185, 129, 0.15); /* свечение для карточек gold */
  --color-amber:         #f59e0b;   /* янтарный — предупреждения, истекающие подписки */
  --color-amber-dark:    #d97706;
  --color-red:           #ef4444;   /* красный — ошибки, истёкшие, danger */
  --color-red-dark:      #dc2626;
  --color-blue:          #3b82f6;   /* синий — fallback carriers, info */

  /* === Текст === */
  --color-text-primary:  #f1f5f9;   /* почти белый — основной текст */
  --color-text-secondary:#94a3b8;   /* серо-голубой — вторичный текст, подписи */
  --color-text-muted:    #64748b;   /* приглушённый — неактивный текст */

  /* === Границы и разделители === */
  --color-border:        #334155;   /* граница карточек */
  --color-border-focus:  #10b981;   /* граница при фокусе input */

  /* === Статусы клиентов === */
  --color-status-active:    #10b981;  /* зелёный */
  --color-status-expiring:  #f59e0b;  /* жёлтый */
  --color-status-expired:   #ef4444;  /* красный */
  --color-status-suspended: #64748b;  /* серый */

  /* === Тени === */
  --shadow-card:    0 4px 6px -1px rgba(0,0,0,0.3), 0 2px 4px -1px rgba(0,0,0,0.2);
  --shadow-modal:   0 20px 60px -10px rgba(0,0,0,0.8);
  --shadow-emerald: 0 0 20px rgba(16, 185, 129, 0.3);

  /* === Анимации === */
  --transition-fast:   0.15s ease;
  --transition-normal: 0.25s ease;
  --transition-slow:   0.4s ease;

  /* === Размеры === */
  --radius-sm:  6px;
  --radius-md:  10px;
  --radius-lg:  16px;
  --radius-xl:  24px;

  /* === Отступы === */
  --sidebar-width: 240px;
  --topbar-height: 60px;
  --content-padding: 24px;
}
```

### 6.2.2 Типографика

```css
/* Шрифты — системный стек (нет внешних CDN для текста) */
body {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto,
               "Helvetica Neue", Arial, sans-serif;
  font-size: 14px;
  line-height: 1.5;
  color: var(--color-text-primary);
  background-color: var(--color-bg-primary);
}

/* Заголовки */
h1 { font-size: 24px; font-weight: 700; }
h2 { font-size: 20px; font-weight: 600; }
h3 { font-size: 16px; font-weight: 600; }
h4 { font-size: 14px; font-weight: 600; }

/* Код/URI — моноширинный */
.mono {
  font-family: "JetBrains Mono", "Fira Code", "Cascadia Code",
               "Courier New", monospace;
  font-size: 13px;
}
```

### 6.2.3 Базовые компоненты (CSS-классы)

```css
/* === КАРТОЧКА === */
.card {
  background: var(--color-bg-secondary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  padding: 20px;
  box-shadow: var(--shadow-card);
  transition: border-color var(--transition-fast);
}
.card:hover { border-color: var(--color-bg-tertiary); }
.card--gold {
  border-color: rgba(16, 185, 129, 0.4);
  box-shadow: var(--shadow-emerald);
}

/* === КНОПКИ === */
.btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 8px 16px;
  border-radius: var(--radius-sm);
  font-size: 13px;
  font-weight: 500;
  border: none;
  cursor: pointer;
  transition: all var(--transition-fast);
  text-decoration: none;
}
.btn--primary {
  background: var(--color-emerald);
  color: #fff;
}
.btn--primary:hover { background: var(--color-emerald-dark); transform: translateY(-1px); }
.btn--secondary {
  background: var(--color-bg-tertiary);
  color: var(--color-text-primary);
  border: 1px solid var(--color-border);
}
.btn--danger { background: var(--color-red); color: #fff; }
.btn--ghost {
  background: transparent;
  color: var(--color-text-secondary);
  border: 1px solid var(--color-border);
}
.btn--ghost:hover { color: var(--color-text-primary); border-color: var(--color-bg-tertiary); }
.btn--sm { padding: 4px 10px; font-size: 12px; }
.btn--icon { padding: 6px; border-radius: var(--radius-sm); }
.btn:disabled { opacity: 0.5; cursor: not-allowed; transform: none !important; }

/* === BADGES (статусы) === */
.badge {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}
.badge--active    { background: rgba(16,185,129,0.15);  color: var(--color-emerald); }
.badge--expiring  { background: rgba(245,158,11,0.15);  color: var(--color-amber);   }
.badge--expired   { background: rgba(239,68,68,0.15);   color: var(--color-red);     }
.badge--suspended { background: rgba(100,116,139,0.15); color: var(--color-text-muted); }
.badge--gold      { background: rgba(16,185,129,0.2);   color: var(--color-emerald); }
.badge--fallback  { background: rgba(59,130,246,0.15);  color: var(--color-blue);    }

/* === ПРОГРЕСС-БАР ТРАФИКА === */
.traffic-bar {
  height: 6px;
  border-radius: 3px;
  background: var(--color-bg-tertiary);
  overflow: hidden;
  margin-top: 4px;
}
.traffic-bar__fill {
  height: 100%;
  border-radius: 3px;
  transition: width 0.6s ease;
  background: var(--color-emerald);
}
.traffic-bar__fill--warning { background: var(--color-amber); }   /* > 80% */
.traffic-bar__fill--danger  { background: var(--color-red); }     /* > 95% */

/* === INPUT FIELDS === */
.input {
  width: 100%;
  padding: 10px 14px;
  background: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  color: var(--color-text-primary);
  font-size: 14px;
  transition: border-color var(--transition-fast), box-shadow var(--transition-fast);
  outline: none;
}
.input:focus {
  border-color: var(--color-border-focus);
  box-shadow: 0 0 0 3px rgba(16, 185, 129, 0.1);
}
.input::placeholder { color: var(--color-text-muted); }

/* === ТАБЛИЦА === */
.table { width: 100%; border-collapse: collapse; }
.table th {
  text-align: left;
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--color-text-muted);
  padding: 10px 16px;
  border-bottom: 1px solid var(--color-border);
}
.table td {
  padding: 12px 16px;
  border-bottom: 1px solid rgba(51,65,85,0.5);
  vertical-align: middle;
}
.table tr:hover td { background: rgba(30,41,59,0.5); }
.table tr:last-child td { border-bottom: none; }

/* === TOAST УВЕДОМЛЕНИЯ === */
.toast-container {
  position: fixed;
  bottom: 24px;
  right: 24px;
  z-index: 9999;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.toast {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 16px;
  border-radius: var(--radius-md);
  background: var(--color-bg-secondary);
  border: 1px solid var(--color-border);
  box-shadow: var(--shadow-modal);
  animation: slideUp 0.3s ease;
  min-width: 280px;
  max-width: 400px;
}
.toast--success { border-left: 3px solid var(--color-emerald); }
.toast--error   { border-left: 3px solid var(--color-red); }
.toast--warning { border-left: 3px solid var(--color-amber); }
@keyframes slideUp {
  from { opacity: 0; transform: translateY(16px); }
  to   { opacity: 1; transform: translateY(0); }
}
```

---

## 6.3 Экран: Вход (login.html)

### 6.3.1 Wireframe

```
┌─────────────────────────────────────────────────────────────────────┐
│                                                                     │
│   [Анимированный фон: тонкая сетка + медленно движущиеся линии]    │
│   [CSS-only animation, имитирует зашифрованный трафик]              │
│                                                                     │
│                                                                     │
│         ┌───────────────────────────────────────┐                  │
│         │  [glassmorphism карточка]              │                  │
│         │                                       │                  │
│         │    🔐  olcrtc Panel                   │                  │
│         │    Панель администратора               │                  │
│         │                                       │                  │
│         │   ┌─────────────────────────────────┐ │                  │
│         │   │ 👤  Логин                        │ │                  │
│         │   └─────────────────────────────────┘ │                  │
│         │                                       │                  │
│         │   ┌─────────────────────────────────┐ │                  │
│         │   │ 🔒  Пароль                  👁  │ │                  │
│         │   └─────────────────────────────────┘ │                  │
│         │                                       │                  │
│         │   [❌ Неверный логин или пароль]       │                  │
│         │   (только при ошибке, анимация shake) │                  │
│         │                                       │                  │
│         │   ┌─────────────────────────────────┐ │                  │
│         │   │  🔐  Войти в защищённую зону    │ │                  │
│         │   └─────────────────────────────────┘ │                  │
│         │    (loading spinner при запросе)      │                  │
│         │                                       │                  │
│         │  ─────────────────────────────────── │                  │
│         │  🔒 TLS 1.3  ·  Зашифровано  ·  Только для админа │     │
│         └───────────────────────────────────────┘                  │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### 6.3.2 Полная реализация login.html

```html
<!-- /opt/olcpanel/static/login.html -->
<!DOCTYPE html>
<html lang="ru">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>OlcPanel — Вход</title>
  <link rel="stylesheet" href="/css/style.css">
  <link rel="stylesheet" href="/css/login.css">
</head>
<body class="login-body">

  <!-- Анимированный фон -->
  <div class="login-bg">
    <div class="login-bg__grid"></div>
    <div class="login-bg__lines"></div>
  </div>

  <!-- Карточка входа -->
  <div class="login-container">
    <div class="login-card" id="loginCard">

      <!-- Заголовок -->
      <div class="login-header">
        <div class="login-logo">
          <svg width="32" height="32" viewBox="0 0 32 32" fill="none">
            <!-- Иконка щита с замком -->
            <path d="M16 2L4 7v9c0 7 5.4 13.5 12 15 6.6-1.5 12-8 12-15V7L16 2z"
                  fill="rgba(16,185,129,0.2)" stroke="#10b981" stroke-width="1.5"/>
            <rect x="12" y="14" width="8" height="6" rx="1"
                  fill="#10b981" opacity="0.9"/>
            <circle cx="16" cy="13" r="2.5" fill="none"
                    stroke="#10b981" stroke-width="1.5"/>
          </svg>
        </div>
        <h1 class="login-title">olcrtc Panel</h1>
        <p class="login-subtitle">Панель администратора</p>
      </div>

      <!-- Форма -->
      <div class="login-form" id="loginForm">

        <!-- Поле логина -->
        <div class="form-group">
          <div class="input-wrapper">
            <span class="input-icon">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none"
                   stroke="currentColor" stroke-width="2">
                <path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/>
                <circle cx="12" cy="7" r="4"/>
              </svg>
            </span>
            <input
              type="text"
              id="username"
              class="input input--with-icon"
              placeholder="Логин"
              autocomplete="username"
              autocapitalize="off"
              spellcheck="false"
              maxlength="64"
            >
          </div>
        </div>

        <!-- Поле пароля -->
        <div class="form-group">
          <div class="input-wrapper">
            <span class="input-icon">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none"
                   stroke="currentColor" stroke-width="2">
                <rect x="3" y="11" width="18" height="11" rx="2" ry="2"/>
                <path d="M7 11V7a5 5 0 0 1 10 0v4"/>
              </svg>
            </span>
            <input
              type="password"
              id="password"
              class="input input--with-icon input--with-action"
              placeholder="Пароль"
              autocomplete="current-password"
              maxlength="128"
            >
            <button
              type="button"
              class="input-action"
              id="togglePassword"
              aria-label="Показать/скрыть пароль"
            >
              <!-- Eye icon (показать) -->
              <svg id="eyeShow" width="16" height="16" viewBox="0 0 24 24" fill="none"
                   stroke="currentColor" stroke-width="2">
                <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/>
                <circle cx="12" cy="12" r="3"/>
              </svg>
              <!-- Eye-off icon (скрыть) — скрыт по умолчанию -->
              <svg id="eyeHide" width="16" height="16" viewBox="0 0 24 24" fill="none"
                   stroke="currentColor" stroke-width="2" style="display:none">
                <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94"/>
                <path d="M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19"/>
                <line x1="1" y1="1" x2="23" y2="23"/>
              </svg>
            </button>
          </div>
        </div>

        <!-- Сообщение об ошибке -->
        <div class="error-message" id="errorMessage" style="display:none">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none"
               stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10"/>
            <line x1="12" y1="8" x2="12" y2="12"/>
            <line x1="12" y1="16" x2="12.01" y2="16"/>
          </svg>
          <span id="errorText">Неверный логин или пароль</span>
        </div>

        <!-- Кнопка входа -->
        <button type="button" class="btn btn--login" id="loginBtn">
          <span id="loginBtnText">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none"
                 stroke="currentColor" stroke-width="2">
              <rect x="3" y="11" width="18" height="11" rx="2" ry="2"/>
              <path d="M7 11V7a5 5 0 0 1 10 0v4"/>
            </svg>
            Войти в защищённую зону
          </span>
          <span id="loginBtnSpinner" style="display:none">
            <svg class="spin" width="16" height="16" viewBox="0 0 24 24" fill="none"
                 stroke="currentColor" stroke-width="2">
              <path d="M21 12a9 9 0 1 1-6.219-8.56"/>
            </svg>
            Проверка...
          </span>
        </button>

      </div>

      <!-- Бейджи безопасности -->
      <div class="security-badges">
        <span class="security-badge">
          <svg width="10" height="10" viewBox="0 0 24 24" fill="none"
               stroke="currentColor" stroke-width="2">
            <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>
          </svg>
          TLS 1.3
        </span>
        <span class="security-badge--dot"></span>
        <span class="security-badge">Зашифровано</span>
        <span class="security-badge--dot"></span>
        <span class="security-badge">Только для администраторов</span>
      </div>

    </div>
  </div>

  <script src="/js/login.js"></script>
</body>
</html>
```

### 6.3.3 CSS для страницы входа (login.css)

```css
/* /opt/olcpanel/static/css/login.css */

/* === Фон с анимированной сеткой === */
.login-body {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  position: relative;
}

.login-bg {
  position: fixed;
  inset: 0;
  pointer-events: none;
}

/* Сетка: тонкие линии как зашифрованный трафик */
.login-bg__grid {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(rgba(16,185,129,0.04) 1px, transparent 1px),
    linear-gradient(90deg, rgba(16,185,129,0.04) 1px, transparent 1px);
  background-size: 40px 40px;
}

/* Движущиеся горизонтальные линии */
.login-bg__lines {
  position: absolute;
  inset: 0;
  overflow: hidden;
}
.login-bg__lines::before,
.login-bg__lines::after {
  content: "";
  position: absolute;
  left: -100%;
  width: 200%;
  height: 1px;
  background: linear-gradient(
    90deg,
    transparent 0%,
    rgba(16,185,129,0.3) 50%,
    transparent 100%
  );
  animation: drift 8s linear infinite;
}
.login-bg__lines::after {
  top: 60%;
  animation-delay: -4s;
  animation-duration: 12s;
}
.login-bg__lines::before { top: 30%; }

@keyframes drift {
  from { transform: translateX(-50%); }
  to   { transform: translateX(0%); }
}

/* === Контейнер и карточка === */
.login-container {
  position: relative;
  z-index: 10;
  width: 100%;
  max-width: 400px;
  padding: 16px;
  animation: fadeInScale 0.5s ease;
}

@keyframes fadeInScale {
  from { opacity: 0; transform: scale(0.95) translateY(20px); }
  to   { opacity: 1; transform: scale(1) translateY(0); }
}

.login-card {
  background: rgba(30, 41, 59, 0.8);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border: 1px solid rgba(51, 65, 85, 0.8);
  border-radius: var(--radius-xl);
  padding: 36px 32px;
  box-shadow:
    0 25px 50px -12px rgba(0, 0, 0, 0.8),
    0 0 0 1px rgba(16, 185, 129, 0.05),
    inset 0 1px 0 rgba(255,255,255,0.05);
  transition: box-shadow var(--transition-normal);
}

.login-card:hover {
  box-shadow:
    0 25px 50px -12px rgba(0, 0, 0, 0.8),
    0 0 0 1px rgba(16, 185, 129, 0.1),
    0 0 40px rgba(16, 185, 129, 0.08);
}

/* Shake анимация при ошибке */
.login-card.shake {
  animation: shake 0.5s cubic-bezier(.36,.07,.19,.97) both;
}
@keyframes shake {
  10%, 90%  { transform: translateX(-2px); }
  20%, 80%  { transform: translateX(4px); }
  30%, 50%, 70% { transform: translateX(-6px); }
  40%, 60%  { transform: translateX(6px); }
}

/* === Шапка === */
.login-header { text-align: center; margin-bottom: 28px; }

.login-logo {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 56px;
  height: 56px;
  background: rgba(16, 185, 129, 0.1);
  border: 1px solid rgba(16, 185, 129, 0.3);
  border-radius: var(--radius-lg);
  margin-bottom: 16px;
  box-shadow: 0 0 20px rgba(16, 185, 129, 0.15);
}

.login-title {
  font-size: 22px;
  font-weight: 700;
  color: var(--color-text-primary);
  margin: 0 0 4px;
  letter-spacing: -0.3px;
}

.login-subtitle {
  font-size: 13px;
  color: var(--color-text-muted);
  margin: 0;
}

/* === Поля формы === */
.form-group { margin-bottom: 14px; }

.input-wrapper { position: relative; }

.input-icon {
  position: absolute;
  left: 12px;
  top: 50%;
  transform: translateY(-50%);
  color: var(--color-text-muted);
  pointer-events: none;
  display: flex;
}

.input--with-icon { padding-left: 40px; }
.input--with-action { padding-right: 40px; }

.input-action {
  position: absolute;
  right: 10px;
  top: 50%;
  transform: translateY(-50%);
  background: none;
  border: none;
  color: var(--color-text-muted);
  cursor: pointer;
  padding: 4px;
  display: flex;
  border-radius: 4px;
  transition: color var(--transition-fast);
}
.input-action:hover { color: var(--color-text-primary); }

/* === Ошибка === */
.error-message {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 12px;
  background: rgba(239, 68, 68, 0.1);
  border: 1px solid rgba(239, 68, 68, 0.3);
  border-radius: var(--radius-sm);
  color: var(--color-red);
  font-size: 13px;
  margin-bottom: 14px;
  animation: fadeInScale 0.2s ease;
}

/* === Кнопка входа === */
.btn--login {
  width: 100%;
  justify-content: center;
  padding: 12px 20px;
  font-size: 14px;
  font-weight: 600;
  background: linear-gradient(135deg, #10b981, #059669);
  color: #fff;
  border-radius: var(--radius-sm);
  letter-spacing: 0.2px;
  box-shadow: 0 4px 14px rgba(16, 185, 129, 0.3);
  transition: all var(--transition-fast);
  margin-top: 4px;
}
.btn--login:hover {
  transform: translateY(-1px);
  box-shadow: 0 6px 20px rgba(16, 185, 129, 0.4);
}
.btn--login:active { transform: translateY(0); }

/* === Бейджи безопасности === */
.security-badges {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  margin-top: 24px;
  flex-wrap: wrap;
}

.security-badge {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  color: var(--color-text-muted);
}

.security-badge--dot {
  width: 3px;
  height: 3px;
  border-radius: 50%;
  background: var(--color-text-muted);
  opacity: 0.5;
}

/* === Spinner === */
.spin {
  animation: spin 1s linear infinite;
}
@keyframes spin {
  from { transform: rotate(0deg); }
  to   { transform: rotate(360deg); }
}
```

### 6.3.4 JavaScript для страницы входа (login.js)

```javascript
// /opt/olcpanel/static/js/login.js

(function() {
  'use strict';

  const loginBtn      = document.getElementById('loginBtn');
  const loginBtnText  = document.getElementById('loginBtnText');
  const loginBtnSpinner = document.getElementById('loginBtnSpinner');
  const usernameInput = document.getElementById('username');
  const passwordInput = document.getElementById('password');
  const errorMessage  = document.getElementById('errorMessage');
  const errorText     = document.getElementById('errorText');
  const loginCard     = document.getElementById('loginCard');
  const togglePassBtn = document.getElementById('togglePassword');
  const eyeShow       = document.getElementById('eyeShow');
  const eyeHide       = document.getElementById('eyeHide');

  // === Переключатель видимости пароля ===
  togglePassBtn.addEventListener('click', () => {
    const isPassword = passwordInput.type === 'password';
    passwordInput.type = isPassword ? 'text' : 'password';
    eyeShow.style.display = isPassword ? 'none' : 'block';
    eyeHide.style.display = isPassword ? 'block' : 'none';
  });

  // === Вход по Enter ===
  [usernameInput, passwordInput].forEach(input => {
    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') handleLogin();
    });
  });

  // === Кнопка входа ===
  loginBtn.addEventListener('click', handleLogin);

  async function handleLogin() {
    const username = usernameInput.value.trim();
    const password = passwordInput.value;

    // Базовая валидация
    if (!username) {
      showError('Введите логин');
      usernameInput.focus();
      return;
    }
    if (!password) {
      showError('Введите пароль');
      passwordInput.focus();
      return;
    }

    setLoading(true);
    hideError();

    try {
      const response = await fetch('/api/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',   // важно: для httpOnly cookie
        body: JSON.stringify({ username, password }),
      });

      if (response.ok) {
        // Успешный вход — редиректим на дашборд
        // Небольшая задержка для плавного UX
        loginCard.style.opacity = '0';
        loginCard.style.transform = 'scale(0.98)';
        loginCard.style.transition = 'all 0.3s ease';
        setTimeout(() => {
          window.location.href = '/';
        }, 300);

      } else if (response.status === 429) {
        const data = await response.json();
        const minutes = Math.ceil(data.retry_after_seconds / 60);
        showError(`Слишком много попыток. Подождите ${minutes} мин.`);
        setLoading(false);

      } else {
        // 401 или любая другая ошибка — не раскрываем детали
        showError('Неверный логин или пароль');
        shakeCard();
        passwordInput.value = '';
        passwordInput.focus();
        setLoading(false);
      }

    } catch (err) {
      showError('Ошибка сети. Проверьте подключение.');
      setLoading(false);
    }
  }

  function setLoading(loading) {
    loginBtn.disabled = loading;
    loginBtnText.style.display = loading ? 'none' : 'flex';
    loginBtnSpinner.style.display = loading ? 'flex' : 'none';
  }

  function showError(message) {
    errorText.textContent = message;
    errorMessage.style.display = 'flex';
  }

  function hideError() {
    errorMessage.style.display = 'none';
  }

  function shakeCard() {
    loginCard.classList.remove('shake');
    // reflow для повторного запуска анимации
    void loginCard.offsetWidth;
    loginCard.classList.add('shake');
    loginCard.addEventListener('animationend', () => {
      loginCard.classList.remove('shake');
    }, { once: true });
  }

  // Фокус на поле логина при загрузке
  usernameInput.focus();

})();
```

---

## 6.4 Экран: Layout главного приложения (index.html)

### 6.4.1 Структура SPA

```html
<!-- /opt/olcpanel/static/index.html -->
<!DOCTYPE html>
<html lang="ru">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>OlcPanel</title>
  <link rel="stylesheet" href="/css/style.css">
  <!-- Chart.js с SRI для безопасности -->
  <script
    src="https://cdn.jsdelivr.net/npm/chart.js@4.4.2/dist/chart.umd.min.js"
    integrity="sha384-PLACEHOLDER_HASH"
    crossorigin="anonymous"
    defer>
  </script>
</head>
<body>

  <!-- === SIDEBAR === -->
  <nav class="sidebar" id="sidebar">
    <div class="sidebar__logo">
      <svg width="24" height="24" viewBox="0 0 32 32" fill="none">
        <path d="M16 2L4 7v9c0 7 5.4 13.5 12 15 6.6-1.5 12-8 12-15V7L16 2z"
              fill="rgba(16,185,129,0.2)" stroke="#10b981" stroke-width="1.5"/>
        <rect x="12" y="14" width="8" height="6" rx="1" fill="#10b981" opacity="0.9"/>
        <circle cx="16" cy="13" r="2.5" fill="none" stroke="#10b981" stroke-width="1.5"/>
      </svg>
      <span>olcrtc Panel</span>
    </div>

    <div class="sidebar__nav">
      <a href="#dashboard" class="nav-item" data-view="dashboard">
        <!-- Grid icon -->
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none"
             stroke="currentColor" stroke-width="2">
          <rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/>
          <rect x="14" y="14" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/>
        </svg>
        Дашборд
      </a>
      <a href="#clients" class="nav-item" data-view="clients">
        <!-- Users icon -->
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none"
             stroke="currentColor" stroke-width="2">
          <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/>
          <circle cx="9" cy="7" r="4"/>
          <path d="M23 21v-2a4 4 0 0 0-3-3.87"/>
          <path d="M16 3.13a4 4 0 0 1 0 7.75"/>
        </svg>
        Клиенты
        <span class="nav-badge" id="navClientCount">0</span>
      </a>
      <a href="#carriers" class="nav-item" data-view="carriers">
        <!-- Signal icon -->
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none"
             stroke="currentColor" stroke-width="2">
          <polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/>
        </svg>
        Carriers
        <span class="nav-badge nav-badge--gold" id="navGoldCount">0</span>
      </a>
      <a href="#settings" class="nav-item" data-view="settings">
        <!-- Settings icon -->
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none"
             stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="3"/>
          <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>
        </svg>
        Настройки
      </a>
    </div>

    <div class="sidebar__footer">
      <div class="sidebar__user">
        <div class="user-avatar">A</div>
        <div class="user-info">
          <div class="user-name" id="adminUsername">admin</div>
          <div class="user-role">Администратор</div>
        </div>
      </div>
      <button class="btn btn--ghost btn--sm btn--icon" id="logoutBtn" title="Выйти">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none"
             stroke="currentColor" stroke-width="2">
          <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/>
          <polyline points="16 17 21 12 16 7"/>
          <line x1="21" y1="12" x2="9" y2="12"/>
        </svg>
      </button>
    </div>
  </nav>

  <!-- === MAIN CONTENT === -->
  <main class="main-content" id="mainContent">

    <!-- TOPBAR (мобильный хамбургер + заголовок страницы) -->
    <header class="topbar">
      <button class="btn btn--ghost btn--icon sidebar-toggle" id="sidebarToggle">
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none"
             stroke="currentColor" stroke-width="2">
          <line x1="3" y1="6" x2="21" y2="6"/>
          <line x1="3" y1="12" x2="21" y2="12"/>
          <line x1="3" y1="18" x2="21" y2="18"/>
        </svg>
      </button>
      <h1 class="page-title" id="pageTitle">Дашборд</h1>
      <div class="topbar__actions" id="topbarActions"></div>
    </header>

    <!-- VIEWS — один активен в каждый момент -->
    <div id="view-dashboard" class="view"></div>
    <div id="view-clients"   class="view" style="display:none"></div>
    <div id="view-client"    class="view" style="display:none"></div>
    <div id="view-carriers"  class="view" style="display:none"></div>
    <div id="view-settings"  class="view" style="display:none"></div>

  </main>

  <!-- === MODAL OVERLAY === -->
  <div class="modal-overlay" id="modalOverlay" style="display:none">
    <div class="modal" id="modal">
      <div class="modal__header">
        <h3 class="modal__title" id="modalTitle">Заголовок</h3>
        <button class="btn btn--ghost btn--icon" id="modalClose">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none"
               stroke="currentColor" stroke-width="2">
            <line x1="18" y1="6" x2="6" y2="18"/>
            <line x1="6" y1="6" x2="18" y2="18"/>
          </svg>
        </button>
      </div>
      <div class="modal__body" id="modalBody"></div>
      <div class="modal__footer" id="modalFooter"></div>
    </div>
  </div>

  <!-- === TOAST CONTAINER === -->
  <div class="toast-container" id="toastContainer"></div>

  <!-- Scripts -->
  <script src="/js/api.js"></script>
  <script src="/js/utils.js"></script>
  <script src="/js/charts.js"></script>
  <script src="/js/app.js"></script>
</body>
</html>
```

---

## 6.5 Экран: Дашборд (#dashboard)

### 6.5.1 Wireframe

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  Дашборд                                          [Обновлено: 2 мин назад] │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────────┐  │
│  │ Всего    │  │ Активных │  │Истекает  │  │ Трафик   │  │ OlcRTC     │  │
│  │ клиентов │  │  сейчас  │  │ ≤ 3 дней │  │ сегодня  │  │ ● Running  │  │
│  │    14    │  │    12    │  │    2     │  │ 4.2 ГБ   │  │ uptime 2д  │  │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘  └────────────┘  │
│                                                                             │
│  ┌──────────────────────────────────────┐  ┌─────────────────────────────┐ │
│  │  Требуют внимания (2)                │  │  Трафик за 7 дней           │ │
│  │                                      │  │                             │ │
│  │  ⚠️  Василий — через 2 дня          │  │  15 ─────────────           │ │
│  │      @vasya_v      [Продлить]        │  │  10 ───────────────         │ │
│  │                                      │  │   5 ────────────────        │ │
│  │  🔴 Мария — истекло 5 дней           │  │   0 ───────────────────     │ │
│  │      @masha_m      [Продлить]        │  │     Пн Вт Ср Чт Пт Сб Вс  │ │
│  └──────────────────────────────────────┘  └─────────────────────────────┘ │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 6.5.2 JavaScript рендеринг дашборда

```javascript
// app/static/js/app.js (фрагмент — renderDashboard)

async function renderDashboard() {
  const view = document.getElementById('view-dashboard');
  view.innerHTML = '<div class="loading-skeleton">Загрузка...</div>';

  try {
    const data = await api.get('/api/dashboard');

    view.innerHTML = `
      <!-- KPI виджеты -->
      <div class="kpi-grid">
        ${renderKpiCard('Всего клиентов', data.client_counts.total, 'users')}
        ${renderKpiCard('Активных', data.client_counts.active, 'check-circle', 'emerald')}
        ${renderKpiCard('Истекает ≤ 3 дней', data.alerts.filter(a => a.severity === 'warning').length, 'alert-triangle', 'amber')}
        ${renderKpiCard('Трафик сегодня', formatBytes(data.traffic.today_gb * 1024 * 1024 * 1024), 'activity')}
        ${renderOlcrtcStatusCard(data.olcrtc_status)}
      </div>

      <!-- Секция: Требуют внимания + График -->
      <div class="dashboard-grid">
        ${renderAlertsCard(data.alerts)}
        ${renderTrafficChart(data.traffic)}
      </div>
    `;

    // Инициализируем Chart.js после рендеринга
    if (data.traffic.chart_labels.length > 0) {
      initTrafficChart(
        data.traffic.chart_labels,
        data.traffic.chart_data_gb,
      );
    }

  } catch (err) {
    view.innerHTML = renderError('Не удалось загрузить дашборд', err.message);
  }
}

function renderKpiCard(title, value, icon, color = 'default') {
  return `
    <div class="card kpi-card">
      <div class="kpi-card__icon kpi-card__icon--${color}">
        ${getIcon(icon)}
      </div>
      <div class="kpi-card__body">
        <div class="kpi-card__value">${value}</div>
        <div class="kpi-card__label">${title}</div>
      </div>
    </div>
  `;
}

function renderOlcrtcStatusCard(status) {
  const isRunning = status.status === 'running';
  const dotClass  = isRunning ? 'status-dot--green' : 'status-dot--red';
  const label     = isRunning ? 'Running' : 'Stopped';
  const uptime    = status.uptime_seconds
    ? formatUptime(status.uptime_seconds)
    : '—';

  return `
    <div class="card kpi-card">
      <div class="kpi-card__icon">
        ${getIcon('server')}
      </div>
      <div class="kpi-card__body">
        <div class="kpi-card__value">
          <span class="status-dot ${dotClass}"></span>
          ${label}
        </div>
        <div class="kpi-card__label">OlcRTC · ${uptime}</div>
      </div>
    </div>
  `;
}

function renderAlertsCard(alerts) {
  if (alerts.length === 0) {
    return `
      <div class="card">
        <h3 class="card__title">Требуют внимания</h3>
        <div class="empty-state">
          <div class="empty-state__icon">${getIcon('check-circle')}</div>
          <p>Всё в порядке</p>
        </div>
      </div>
    `;
  }

  const items = alerts.map(alert => `
    <div class="alert-item alert-item--${alert.severity}">
      <div class="alert-item__info">
        <div class="alert-item__name">${escapeHtml(alert.client_name)}</div>
        <div class="alert-item__meta">
          ${alert.telegram ? escapeHtml(alert.telegram) + ' · ' : ''}
          ${alert.message}
        </div>
      </div>
      <button
        class="btn btn--primary btn--sm"
        onclick="openExtendModal(${alert.client_id})"
      >
        Продлить
      </button>
    </div>
  `).join('');

  return `
    <div class="card">
      <div class="card__header">
        <h3 class="card__title">Требуют внимания</h3>
        <span class="badge badge--expired">${alerts.length}</span>
      </div>
      <div class="alerts-list">${items}</div>
    </div>
  `;
}
```

---

## 6.6 Экран: Список клиентов (#clients)

### 6.6.1 Wireframe

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  Клиенты (14)                                    [+ Новый клиент]          │
├─────────────────────────────────────────────────────────────────────────────┤
│  [🔍 Поиск по имени, Telegram...]  [Все статусы ▼]  [По дате окончания ▼] │
├──────┬────────────────┬──────────────┬─────────────────────────┬────────────┤
│      │ Клиент         │ Статус       │ Подписка / Трафик       │ Действия   │
├──────┼────────────────┼──────────────┼─────────────────────────┼────────────┤
│  1   │ Антон          │ ● Активен    │ до 01.07 (19 дн.)       │ [👁][🔑][↻]│
│      │ @anton_vpn     │              │ ███░░░░░░░ 23% / 100 ГБ │            │
├──────┼────────────────┼──────────────┼─────────────────────────┼────────────┤
│  2   │ Василий        │ ⚠️ Истекает  │ до 14.06 (2 дн.)        │ [👁][🔑][↻]│
│      │ @vasya_v       │              │ █████░░░░░ 52% / 30 ГБ  │            │
├──────┼────────────────┼──────────────┼─────────────────────────┼────────────┤
│  3   │ Мария          │ 🔴 Истекло   │ истекло 5 дней назад     │ [👁][🔑][↻]│
│      │ @masha_m       │              │ ████████░░ 87% / 30 ГБ  │            │
├──────┼────────────────┼──────────────┼─────────────────────────┼────────────┤
│  4   │ Игорь          │ ⏸ Приостан. │ приостановлен вручную    │ [👁][🔑][▶]│
│      │ —              │              │ ██░░░░░░░░ 18% / 100 ГБ │            │
└──────┴────────────────┴──────────────┴─────────────────────────┴────────────┘
│  ← 1-14 из 14 клиентов →                                                   │
└─────────────────────────────────────────────────────────────────────────────┘

Действия: [👁] = карточка, [🔑] = показать конфиг, [↻] = продлить, [▶] = восстановить
```

### 6.6.2 JavaScript рендеринг списка клиентов

```javascript
// app/static/js/app.js (фрагмент — renderClients)

let clientsState = {
  search: '',
  status: '',
  sortBy: 'expires_at',
  sortDir: 'asc',
  skip: 0,
  limit: 50,
};

async function renderClients() {
  const view = document.getElementById('view-clients');

  // Панель инструментов
  view.innerHTML = `
    <div class="toolbar">
      <div class="toolbar__search">
        <div class="input-wrapper">
          <span class="input-icon">${getIcon('search')}</span>
          <input
            type="text"
            id="clientSearch"
            class="input input--with-icon"
            placeholder="Поиск по имени, Telegram..."
            value="${escapeHtml(clientsState.search)}"
          >
        </div>
      </div>
      <div class="toolbar__filters">
        <select class="input input--select" id="clientStatusFilter">
          <option value="">Все статусы</option>
          <option value="active">Активные</option>
          <option value="expiring">Истекают</option>
          <option value="expired">Истекло</option>
          <option value="suspended">Приостановлены</option>
        </select>
        <select class="input input--select" id="clientSortBy">
          <option value="expires_at">По дате окончания</option>
          <option value="name">По имени</option>
          <option value="created_at">По дате создания</option>
        </select>
      </div>
      <button class="btn btn--primary" onclick="openCreateClientModal()">
        ${getIcon('plus')}
        Новый клиент
      </button>
    </div>

    <div class="card" style="padding:0; overflow:hidden;">
      <div id="clientsTableContainer">
        <div class="loading-skeleton" style="padding:40px; text-align:center">
          Загрузка клиентов...
        </div>
      </div>
    </div>
  `;

  // Live search
  const searchInput = document.getElementById('clientSearch');
  let searchTimer;
  searchInput.addEventListener('input', (e) => {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(() => {
      clientsState.search = e.target.value;
      clientsState.skip = 0;
      loadClientsTable();
    }, 300);
  });

  document.getElementById('clientStatusFilter').addEventListener('change', (e) => {
    clientsState.status = e.target.value;
    clientsState.skip = 0;
    loadClientsTable();
  });

  await loadClientsTable();
}

async function loadClientsTable() {
  const container = document.getElementById('clientsTableContainer');
  if (!container) return;

  try {
    const params = new URLSearchParams({
      skip: clientsState.skip,
      limit: clientsState.limit,
      sort_by: clientsState.sortBy,
      sort_dir: clientsState.sortDir,
    });
    if (clientsState.search) params.set('search', clientsState.search);
    if (clientsState.status) params.set('status', clientsState.status);

    const data = await api.get(`/api/clients?${params}`);

    if (data.items.length === 0) {
      container.innerHTML = `
        <div class="empty-state" style="padding:60px">
          ${getIcon('users', 40)}
          <p>Клиенты не найдены</p>
          ${!clientsState.search && !clientsState.status
            ? '<button class="btn btn--primary" onclick="openCreateClientModal()">Создать первого клиента</button>'
            : ''}
        </div>
      `;
      return;
    }

    const rows = data.items.map(client => {
      const sub = client.active_subscription;
      const statusBadge = renderStatusBadge(client.status, sub);
      const trafficBar = sub ? renderTrafficBar(sub) : '—';
      const subscriptionInfo = sub
        ? `до ${formatDate(sub.expires_at)} (${sub.days_left} дн.)`
        : 'Нет подписки';

      return `
        <tr class="table-row--clickable" onclick="navigateTo('client', ${client.id})">
          <td>
            <div class="client-name">${escapeHtml(client.name)}</div>
            <div class="client-meta">${client.telegram ? escapeHtml(client.telegram) : '—'}</div>
          </td>
          <td>${statusBadge}</td>
          <td>
            <div class="subscription-info">${subscriptionInfo}</div>
            <div style="margin-top:4px; min-width:140px">${trafficBar}</div>
          </td>
          <td>
            <div class="action-buttons" onclick="event.stopPropagation()">
              <button
                class="btn btn--ghost btn--icon"
                onclick="navigateTo('client', ${client.id})"
                title="Открыть карточку"
              >${getIcon('eye')}</button>
              <button
                class="btn btn--ghost btn--icon"
                onclick="openConfigModal(${client.id})"
                title="Показать конфиг"
              >${getIcon('key')}</button>
              <button
                class="btn btn--primary btn--sm"
                onclick="openExtendModal(${client.id})"
                title="Продлить подписку"
              >${getIcon('refresh-cw')}</button>
            </div>
          </td>
        </tr>
      `;
    }).join('');

    container.innerHTML = `
      <table class="table">
        <thead>
          <tr>
            <th>Клиент</th>
            <th>Статус</th>
            <th>Подписка / Трафик</th>
            <th>Действия</th>
          </tr>
        </thead>
        <tbody>${rows}</tbody>
      </table>
      <div class="table-footer">
        ${data.total} клиентов
      </div>
    `;

  } catch (err) {
    container.innerHTML = renderError('Ошибка загрузки', err.message);
  }
}

function renderTrafficBar(sub) {
  const percent = sub.traffic_used_percent;
  const limitStr = sub.traffic_limit_mb === 0 ? 'безлимит' : `${(sub.traffic_limit_mb/1024).toFixed(0)} ГБ`;
  const usedStr = `${(sub.traffic_used_mb/1024).toFixed(1)} ГБ`;

  if (sub.traffic_limit_mb === 0) {
    return `<div class="traffic-label">${usedStr} / ${limitStr}</div>`;
  }

  const fillClass = percent >= 95 ? 'traffic-bar__fill--danger'
                  : percent >= 80 ? 'traffic-bar__fill--warning'
                  : '';

  return `
    <div>
      <div class="traffic-label">${usedStr} / ${limitStr} (${percent}%)</div>
      <div class="traffic-bar">
        <div
          class="traffic-bar__fill ${fillClass}"
          style="width: ${Math.min(percent, 100)}%"
        ></div>
      </div>
    </div>
  `;
}
```

---

## 6.7 Экран: Карточка клиента (#clients/{id})

### 6.7.1 Wireframe

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  ← Клиенты  /  Антон                         [Приостановить] [Удалить]    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌────────────────────────────────┐  ┌──────────────────────────────────┐  │
│  │  ОСНОВНАЯ ИНФОРМАЦИЯ           │  │  ПОДПИСКА                        │  │
│  │                                │  │                                  │  │
│  │  Имя:      Антон               │  │  Тариф:   Стандарт               │  │
│  │  Telegram: @anton_vpn          │  │  Начало:  01.06.2026             │  │
│  │  Телефон:  +7 916 123 45 67    │  │  Конец:   01.07.2026 (19 дн.)   │  │
│  │  Заметка:  Друг Миши           │  │                                  │  │
│  │  Создан:   01.06.2026          │  │  Трафик: 23.4 / 100 ГБ (23%)    │  │
│  │                                │  │  ████░░░░░░░░░░░░░░              │  │
│  │  [Редактировать]               │  │                                  │  │
│  └────────────────────────────────┘  │  [Продлить подписку]             │  │
│                                       └──────────────────────────────────┘  │
│  ┌────────────────────────────────────────────────────────────────────────┐  │
│  │  КОНФИГ OLCRTC                                                        │  │
│  │                                                                        │  │
│  │  URI:  olcrtc://jitsi?datachannel@https://meet1.../olcrtc-a1b2c3d4#.. │  │
│  │        [📋 Копировать URI]  [📥 Скачать YAML]  [🔁 Сменить конфиг]   │  │
│  │                                                                        │  │
│  │  ┌──────────────┐                                                      │  │
│  │  │  [QR-КОД]    │  Отсканируйте в OlcBox для подключения              │  │
│  │  │  (200x200)   │  или скопируйте URI вручную                         │  │
│  │  └──────────────┘                                                      │  │
│  └────────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
│  ┌──────────────────────────────┐  ┌─────────────────────────────────────┐ │
│  │  ТРАФИК (30 дней)            │  │  ИСТОРИЯ ПЛАТЕЖЕЙ                   │ │
│  │  [линейный график Chart.js]  │  │  12.06 — 500₽ карта    [Продлить]  │ │
│  │                              │  │  01.06 — 500₽ карта                 │ │
│  └──────────────────────────────┘  └─────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 6.8 Модальные окна

### 6.8.1 Модал: Создать клиента

```javascript
function openCreateClientModal() {
  openModal('Новый клиент', `
    <div class="form-grid">
      <div class="form-group">
        <label class="form-label">Имя *</label>
        <input type="text" id="newClientName" class="input" placeholder="Антон" maxlength="128">
      </div>
      <div class="form-group">
        <label class="form-label">Telegram</label>
        <input type="text" id="newClientTelegram" class="input" placeholder="@username" maxlength="128">
      </div>
      <div class="form-group">
        <label class="form-label">Телефон</label>
        <input type="tel" id="newClientPhone" class="input" placeholder="+7 916 123 45 67" maxlength="32">
      </div>
      <div class="form-group">
        <label class="form-label">Тариф *</label>
        <select id="newClientPlan" class="input input--select">
          <option value="">Загрузка...</option>
        </select>
      </div>
      <div class="form-group form-group--full">
        <label class="form-label">Заметки</label>
        <textarea id="newClientNotes" class="input" rows="2" maxlength="1000"
                  placeholder="Откуда клиент, как платит..."></textarea>
      </div>
    </div>
  `, [
    { label: 'Отмена', class: 'btn--ghost', action: closeModal },
    { label: 'Создать клиента', class: 'btn--primary', action: submitCreateClient, id: 'submitCreate' },
  ]);

  // Загружаем тарифы
  loadPlansIntoSelect('newClientPlan');
}

async function submitCreateClient() {
  const btn = document.getElementById('submitCreate');
  const name     = document.getElementById('newClientName').value.trim();
  const telegram = document.getElementById('newClientTelegram').value.trim();
  const phone    = document.getElementById('newClientPhone').value.trim();
  const plan_id  = parseInt(document.getElementById('newClientPlan').value);
  const notes    = document.getElementById('newClientNotes').value.trim();

  if (!name)    { showModalError('Введите имя клиента'); return; }
  if (!plan_id) { showModalError('Выберите тариф'); return; }

  btn.disabled = true;
  btn.textContent = 'Создание...';

  try {
    const client = await api.post('/api/clients', {
      name, telegram: telegram || null, phone: phone || null,
      notes: notes || null, plan_id,
    });

    closeModal();
    showToast('success', `Клиент "${client.name}" создан`);

    // Показываем конфиг сразу
    if (client.active_config) {
      openConfigModal(client.id);
    }

    // Обновляем список
    await loadClientsTable();

  } catch (err) {
    showModalError(err.message || 'Ошибка создания клиента');
    btn.disabled = false;
    btn.textContent = 'Создать клиента';
  }
}
```

### 6.8.2 Модал: Показать конфиг (URI + QR)

```javascript
async function openConfigModal(clientId) {
  openModal('Конфиг OlcRTC', '<div class="loading-skeleton">Загрузка конфига...</div>', []);

  try {
    const config = await api.get(`/api/clients/${clientId}/config`);

    document.getElementById('modalBody').innerHTML = `
      <!-- URI -->
      <div class="config-uri-block">
        <label class="form-label">OlcRTC URI</label>
        <div class="uri-display">
          <code class="uri-text mono" id="configUri">${escapeHtml(config.uri)}</code>
          <button class="btn btn--primary btn--sm" onclick="copyToClipboard('${escapeHtml(config.uri)}', this)">
            ${getIcon('copy')}
            Копировать
          </button>
        </div>
      </div>

      <!-- QR-код -->
      <div class="qr-block">
        <div class="qr-code">
          <img
            src="data:image/png;base64,${config.qr_code_base64}"
            alt="QR-код конфига OlcRTC"
            width="200"
            height="200"
            style="border-radius:8px"
          >
        </div>
        <div class="qr-instructions">
          <p><strong>Как использовать:</strong></p>
          <ol>
            <li>Установите <strong>OlcBox</strong> (Android/iOS)</li>
            <li>Откройте приложение → Import</li>
            <li>Отсканируйте QR-код <em>или</em> вставьте URI</li>
            <li>Нажмите Connect</li>
          </ol>
        </div>
      </div>

      <!-- Действия -->
      <div class="config-actions">
        <a href="/api/clients/${clientId}/config/download"
           class="btn btn--ghost btn--sm" download>
          ${getIcon('download')}
          Скачать YAML
        </a>
        <button class="btn btn--ghost btn--sm"
                onclick="openQrFullscreen('${config.qr_code_base64}')">
          ${getIcon('maximize')}
          QR на весь экран
        </button>
        <button class="btn btn--danger btn--sm"
                onclick="openRevokeConfigModal(${clientId}, ${config.id})">
          ${getIcon('refresh-cw')}
          Сменить конфиг
        </button>
      </div>
    `;

    // Обновляем кнопки модала
    document.getElementById('modalFooter').innerHTML = `
      <button class="btn btn--ghost" onclick="closeModal()">Закрыть</button>
    `;

  } catch (err) {
    document.getElementById('modalBody').innerHTML = renderError('Ошибка загрузки конфига');
  }
}

async function copyToClipboard(text, btn) {
  try {
    await navigator.clipboard.writeText(text);
    const original = btn.innerHTML;
    btn.innerHTML = `${getIcon('check')} Скопировано!`;
    btn.classList.add('btn--success');
    setTimeout(() => {
      btn.innerHTML = original;
      btn.classList.remove('btn--success');
    }, 2000);
  } catch (err) {
    showToast('error', 'Не удалось скопировать');
  }
}
```

---

## 6.9 Утилиты (utils.js)

```javascript
// /opt/olcpanel/static/js/utils.js

// === Безопасное экранирование HTML (защита от XSS) ===
function escapeHtml(str) {
  if (str === null || str === undefined) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}
// ВАЖНО: никогда не использовать innerHTML с неэкранированными данными от сервера

// === Форматирование ===
function formatBytes(bytes) {
  if (bytes === 0) return '0 Б';
  const k = 1024;
  const sizes = ['Б', 'КБ', 'МБ', 'ГБ', 'ТБ'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

function formatDate(iso) {
  if (!iso) return '—';
  const d = new Date(iso);
  return d.toLocaleDateString('ru-RU', { day:'2-digit', month:'2-digit', year:'numeric' });
}

function formatUptime(seconds) {
  if (seconds < 60)   return `${seconds}с`;
  if (seconds < 3600) return `${Math.floor(seconds/60)}м`;
  if (seconds < 86400)return `${Math.floor(seconds/3600)}ч`;
  return `${Math.floor(seconds/86400)}д ${Math.floor((seconds%86400)/3600)}ч`;
}

// === Toast уведомления ===
function showToast(type, message, duration = 4000) {
  const container = document.getElementById('toastContainer');
  const toast = document.createElement('div');
  toast.className = `toast toast--${type}`;
  toast.innerHTML = `
    <span>${type === 'success' ? getIcon('check-circle') : type === 'error' ? getIcon('x-circle') : getIcon('alert-triangle')}</span>
    <span>${escapeHtml(message)}</span>
  `;
  container.appendChild(toast);
  setTimeout(() => {
    toast.style.opacity = '0';
    toast.style.transform = 'translateY(8px)';
    toast.style.transition = 'all 0.3s ease';
    setTimeout(() => toast.remove(), 300);
  }, duration);
}

// === Модальное окно ===
function openModal(title, bodyHtml, buttons = []) {
  document.getElementById('modalTitle').textContent = title;
  document.getElementById('modalBody').innerHTML = bodyHtml;

  const footer = document.getElementById('modalFooter');
  footer.innerHTML = buttons.map(btn => `
    <button
      class="btn ${btn.class || 'btn--ghost'}"
      ${btn.id ? `id="${btn.id}"` : ''}
      onclick="(${btn.action.toString()})()"
    >${btn.label}</button>
  `).join('');

  const overlay = document.getElementById('modalOverlay');
  overlay.style.display = 'flex';
  setTimeout(() => overlay.classList.add('modal-overlay--visible'), 10);
}

function closeModal() {
  const overlay = document.getElementById('modalOverlay');
  overlay.classList.remove('modal-overlay--visible');
  setTimeout(() => { overlay.style.display = 'none'; }, 250);
}

// === API клиент ===
// (в api.js)
const api = {
  async get(url) {
    const res = await fetch(url, { credentials: 'include' });
    if (res.status === 401) { window.location.href = '/login'; throw new Error('Unauthorized'); }
    if (!res.ok) { const err = await res.json().catch(() => ({})); throw new Error(err.detail || `HTTP ${res.status}`); }
    return res.json();
  },
  async post(url, body) {
    const res = await fetch(url, { method:'POST', credentials:'include',
      headers:{'Content-Type':'application/json'}, body: JSON.stringify(body) });
    if (res.status === 401) { window.location.href = '/login'; throw new Error('Unauthorized'); }
    if (!res.ok) { const err = await res.json().catch(() => ({})); throw new Error(err.detail || `HTTP ${res.status}`); }
    return res.json();
  },
  async patch(url, body) {
    const res = await fetch(url, { method:'PATCH', credentials:'include',
      headers:{'Content-Type':'application/json'}, body: JSON.stringify(body) });
    if (res.status === 401) { window.location.href = '/login'; throw new Error('Unauthorized'); }
    if (!res.ok) { const err = await res.json().catch(() => ({})); throw new Error(err.detail || `HTTP ${res.status}`); }
    return res.json();
  },
  async delete(url) {
    const res = await fetch(url, { method:'DELETE', credentials:'include' });
    if (res.status === 401) { window.location.href = '/login'; throw new Error('Unauthorized'); }
    if (!res.ok) { const err = await res.json().catch(() => ({})); throw new Error(err.detail || `HTTP ${res.status}`); }
    return res.status === 204 ? null : res.json();
  },
};
```

---

## 6.10 Адаптивность (Mobile)

```css
/* Мобильный layout — sidebar скрывается, появляется hamburger */

@media (max-width: 768px) {
  .sidebar {
    position: fixed;
    left: -var(--sidebar-width);
    top: 0;
    bottom: 0;
    z-index: 1000;
    transition: left var(--transition-normal);
    box-shadow: none;
  }
  .sidebar.sidebar--open {
    left: 0;
    box-shadow: 4px 0 20px rgba(0,0,0,0.5);
  }

  .main-content {
    margin-left: 0;
  }

  .sidebar-toggle { display: flex !important; }

  .kpi-grid {
    grid-template-columns: repeat(2, 1fr);
  }

  .dashboard-grid {
    grid-template-columns: 1fr;
  }

  .table th:nth-child(3),
  .table td:nth-child(3) { display: none; }  /* скрываем трафик на мобиле */

  .config-uri-block .uri-text {
    font-size: 10px;
    word-break: break-all;
  }
}

@media (max-width: 480px) {
  .kpi-grid { grid-template-columns: 1fr 1fr; }
  .login-card { padding: 24px 20px; }
  .content-padding { padding: 12px; }
}
```

---

## 6.10 v2.0-элементы UI (поверх LEGACY-вайрфреймов выше)

### 6.10.1 Дашборд: виджет «Свежесть доступа / Флот»

Заменяет KPI «Трафик» (Решение 5). Источник — `GET /api/servers` (4.11) + `authz_state`.

```
┌──────────────────────────────────────────────────────────────────────┐
│  Доступ (authz)                                                        │
│  Версия allowlist: v42 · устройств в allow: 12 · обновлено 1 мин назад │
├──────────────────────────────────────────────────────────────────────┤
│  Сервер     Статус   applied/expected   LKG возраст   ошибки  health   │
│  default    ● active   42 / 42            1 мин         0       ок      │
│  vps-eu-2   ▲ stale    40 / 42 (-2)       3 ч 12 м      0     ⚠ устар.  │   ← «Требуют внимания»
│  vps-eu-3   ✖ errors   38 / 42            —             7     ⚠ битый   │   ← алерт
└──────────────────────────────────────────────────────────────────────┘
```

Правила индикации (прямо из last-known-good, 5.2):
- `applied == expected`, `load_errors=0`, свежий `lkg_valid_at` → зелёный «ок».
- `applied < expected` → жёлтый «отстаёт» (доставка не подтверждена / split-brain, 5.3).
- `now - lkg_valid_at > lkg_max_age` → оранжевый «устаревший allowlist» (`stale:true` из `/health`): сервер жив, но применяет старый-но-валидный срез.
- `load_errors > 0` → красный «битый файл»: `Gate` работает по LKG, нужен re-push (кнопка → `POST /api/servers/{id}/repush-authz`, 4.11).
Все жёлтые/оранжевые/красные строки также попадают в блок **«Требуют внимания»** рядом с истекающими подписками.

### 6.10.2 Карточка клиента: секция «Устройства» (вместо «Сменить конфиг»)

Источник — `GET /api/clients/{id}/devices` (4.10).

```
┌─────────────────────────────────────────────────────────────────┐
│  Устройства                                          [нет лимита] │
│  ───────────────────────────────────────────────────────────────│
│  📱 телефон Антона   hwid-aaa…   сервер: default                  │
│       посл. активность: 2 ч назад   ● в allow      [Отвязать]     │
│  💻 ноутбук          hwid-bbb…   сервер: default                  │
│       посл. активность: —          ○ не в allow    [Отвязать]     │
│  ───────────────────────────────────────────────────────────────│
│  «Отвязать» убирает deviceId из authz.json (остальные клиенты не  │
│  затронуты — общий ключ). Блок клиента снимает все его устройства.│
└─────────────────────────────────────────────────────────────────┘
```

### 6.10.3 Карточка клиента: «Ссылка подписки» (вместо пер-клиентского конфига)

```
┌─────────────────────────────────────────────────────────────────┐
│  Подписка для OlcBox                                             │
│  [ olcrtc://jitsi?datachannel@…общая комната…#…общий ключ… ]     │
│  [📋 Копировать ссылку]  [🔁 Ротация sub-токена]   [ QR ]        │
│  Комната и ключ — общие (server.yaml). Различие клиентов — по    │
│  устройству. «Ротация» рвёт старую ссылку (/api/sub/{old}→404),  │
│  активные устройства не трогает.                                  │
└─────────────────────────────────────────────────────────────────┘
```

### 6.10.4 Список клиентов: колонка «Подписка / Устройства»

Колонка «Трафик» (LEGACY-вайрфреймы 6.6) заменяется на число устройств и статус: `до 01.07 · 2 устр. · ● активен`. Трафик не отображается (учёт не настроен).

---

*Следующий раздел: [[OlcPanel_07_Инфраструктура]] — Nginx-конфиг, systemd unit-файлы, Let's Encrypt, доставка authz, рестарт с проверкой файла, бэкап.*

════════════════════════════════════════════════════════════════════════════════
<!-- Конец файла 06_UI_Карта_экранов.md -->
════════════════════════════════════════════════════════════════════════════════
