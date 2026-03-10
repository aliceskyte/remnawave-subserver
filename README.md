# Remnawave Subserver

`Remnawave Subserver` — это внешнее расширение для Remnawave.

Он нужен для сценариев, в которых самой Remnawave неудобно или недостаточно
ее встроенного механизма шаблонов: сложные конфиги, составные шаблоны,
отдельная логика для `xray` и `mihomo`, squad-specific выдача и тонкая
настройка subscription headers.

Сервис принимает пользовательский `shortUUID`, забирает данные из панели
Remnawave, накладывает локальные шаблоны и отдает готовую подписку для
`xray` или `mihomo`.

В проект также входит простая admin UI / admin API для управления шаблонами
конфигов и HTTP-хедерами подписки.

## Зачем он нужен

Remnawave хорошо закрывает базовую панельную логику, но для сложных клиентских
конфигов этого часто недостаточно.

`Remnawave Subserver` решает именно этот слой и выносит сложную логику
шаблонов и выдачи подписок за пределы самой панели.

## Что умеет

- работает как внешний subserver поверх Remnawave
- отдает подписки по `shortUUID`
- поддерживает два core: `xray` и `mihomo`
- хранит шаблоны и header overrides в SQLite
- поддерживает `default`-шаблоны и отдельные шаблоны по squad UUID
- автоматически подставляет персональный `vlessUuid` пользователя
- собирает один импортируемый Mihomo YAML из нескольких standalone-профилей
- дает web-admin для редактирования шаблонов и хедеров
- умеет bootstrap начальных шаблонов из `configs/default.json`

## Как это работает

1. Клиент запрашивает `/{shortUUID}` или `/{shortUUID}?core=mihomo`
2. `Remnawave Subserver` обращается в панель и получает:
   - данные пользователя
   - raw subscription / raw headers
3. Сервис выбирает шаблон:
   - squad-шаблон, если совпал первый активный squad UUID
   - иначе `default`
4. Шаблон персонализируется:
   - VLESS UUID подменяется на реальный `vlessUuid` пользователя
   - подставляются placeholders
   - применяются header overrides
5. Готовый результат возвращается как JSON (`xray`) или YAML (`mihomo`)

## Поддерживаемые core

### `xray`

- шаблоны хранятся как JSON
- для VLESS outbounds автоматически переписывается `users[].id`
- поддерживаются пользовательские `remarks`
- в `remarks` можно использовать `{user}` и `{username}`

### `mihomo`

- шаблоны хранятся как YAML-текст
- поддерживаются `{user}`, `{username}` и `{vless_uuid}`
- если шаблон состоит из нескольких standalone Mihomo-профилей, `subserver`
  собирает из них один импортируемый YAML:
  - общий список `proxies`
  - одна группа `proxy-groups: PROXY`
  - сохраняются `DIRECT` / `REJECT` правила
  - в конец добавляется `MATCH,PROXY`
- если Mihomo-шаблоны отсутствуют, они автоматически генерируются из Xray

## Публичные endpoints

### Healthcheck

```http
GET /health
```

Возвращает `200 OK`.

### Выдача подписки

```http
GET /{shortUUID}
GET /{shortUUID}?core=xray
GET /{shortUUID}?core=mihomo
```

Поведение:

- по умолчанию используется `xray`
- некорректный `core` дает `400`
- некорректный `shortUUID` дает `400`
- неизвестный пользователь дает `404`

Пример:

```bash
curl "http://127.0.0.1:8080/0cb6c0a7beed?core=mihomo"
```

Legacy-формат ссылки тоже поддерживается:

```text
/0cb6c0a7beed&core=mihomo
```

## Админка

UI доступен по адресу:

```text
/admin/
```

Авторизация локальная и отделена от panel token.

Используется `ADMIN_TOKEN`:

```http
Authorization: Bearer <ADMIN_TOKEN>
```

Этот же токен использует admin UI.

## Admin API

### Проверка токена

```http
GET /admin/api/auth/check
```

### Шаблоны конфигов

```http
GET  /admin/api/configs
POST /admin/api/configs
```

### Header overrides

```http
GET  /admin/api/headers
POST /admin/api/headers
```

### Вспомогательные методы для панели

```http
GET /admin/api/remnawave/internal-squads
GET /admin/api/remnawave/headers?uuid=<shortUUID>
```

## Формат библиотеки шаблонов

Шаблоны разделены по core:

```json
{
  "xray": {
    "default": [],
    "squads": {}
  },
  "mihomo": {
    "default": [],
    "squads": {}
  }
}
```

У каждого core есть:

- `default` — шаблоны по умолчанию
- `squads` — шаблоны, привязанные к UUID internal squad

Важно: выбор squad-шаблона идет по первому активному squad UUID, который
вернула панель. Если для него есть шаблон, используется он. Если нет —
используется `default`.

## Особенности шаблонов

### Служебный блок `subserver` в Xray-конфигах

В Xray-шаблоне можно использовать служебный блок:

```json
{
  "subserver": {
    "skipOutboundTags": ["proxy", "static-node"],
    "randomize": {
      "proxy": ["proxy_a", "proxy_b"],
      "media": ["media_a", "media_b"]
    }
  }
}
```

Этот блок:

- нужен только самому `Remnawave Subserver`
- не попадает в финальный ответ клиенту

#### `skipOutboundTags`

`skipOutboundTags` говорит `Remnawave Subserver`, что для указанных outbound
tag не нужно переписывать VLESS UUID.

Это полезно, если какой-то outbound должен всегда оставаться со статическим
UUID, а не с персональным `vlessUuid` пользователя.

Важно: эта логика относится именно к Xray JSON builder.

#### `randomize`

`randomize` задает группы в формате:

- ключ — итоговый `outbounds[].tag`, который должен остаться в финальном конфиге
- значение — список candidate `outbounds[].tag`, между которыми нужно выбрать один

На каждом запросе `Remnawave Subserver`:

- случайно выбирает один реальный outbound tag для каждой группы
- если выбран candidate с другим tag, переименовывает его в ключ группы
- удаляет остальные candidate-outbound из финального Xray JSON

Ограничения:

- один реальный outbound tag не должен попадать в несколько групп
- если tag из ключа уже существует в `outbounds`, его нужно включить в список candidate
- логика применяется только к Xray JSON builder

### Placeholders в Xray

Поддерживаются в `remarks`:

- `{user}`
- `{username}`

### Placeholders в Mihomo

Поддерживаются внутри YAML:

- `{user}`
- `{username}`
- `{vless_uuid}`

## Header overrides

`subserver` умеет изменять хедеры, которые приходят из панели вместе с raw
подпиской.

Типичные ключи:

- `profile-title`
- `profile-update-interval`
- `subscription-userinfo`
- `support-url`
- `profile-web-page-url`

Overrides хранятся:

- отдельно по core
- отдельно для `default`
- отдельно по squad

Для `subscription-userinfo` есть параметрический режим управления:

- `upload`
- `download`
- `total`
- `expire`

Режимы:

- `actual` — оставить фактическое значение панели
- `custom` — подменить своим значением
- `remove` — удалить поле или хедер

## Хранение данных

Во время работы сервис использует SQLite.

В базе лежат:

- шаблоны конфигов
- header overrides

Важно:

- `configs/default.json` нужен только как bootstrap-источник
- после первого bootstrap изменения сохраняются уже в SQLite
- в production bootstrap-файл можно монтировать только на чтение

## Переменные окружения

Обязательные:

- `PANEL_URL`
- `PANEL_TOKEN`
- `ADMIN_TOKEN`

Часто используемые optional:

- `SUB_PATH_PREFIX`
- `SUBSERVER_BIND`
- `SUBSERVER_PORT`
- `PANEL_CACHE_TTL`
- `RATE_LIMIT_RPS`
- `RATE_LIMIT_BURST`
- `ADMIN_RATE_LIMIT_RPS`
- `ADMIN_RATE_LIMIT_BURST`
- `TRUSTED_PROXY_CIDRS`
- `SUBSERVER_DB_PATH`
- `SUBSERVER_DB_DSN`

Полный список смотри в [.env.example](.env.example).

## Быстрый старт

### Локальный запуск

```bash
go run .
```

### Docker

```bash
docker compose up -d --build
```

Подробности по production-деплою смотри в [DEPLOY.md](DEPLOY.md).

## Production notes

- админка использует локальный `ADMIN_TOKEN`, а не panel token
- контейнер запускается не от `root`
- root filesystem у контейнера read-only
- bootstrap-конфиг монтируется только на чтение
- public API и admin API ограничены rate limit
- `X-Forwarded-For` / `X-Real-IP` доверяются только явно разрешенным proxy CIDR
- локальный `.env` приложением в production не подхватывается, если явно не
  включить `SUBSERVER_LOAD_DOTENV=1`

## Структура репозитория

```text
.
├── admin/                  # admin UI
├── cmd/                    # вспомогательные команды
├── configs/                # bootstrap templates
├── internal/
│   ├── admin/              # admin HTTP server
│   ├── adminstate/         # хранение header overrides
│   ├── config/             # парсинг и сборка шаблонов
│   ├── db/                 # миграции
│   ├── handler/            # public HTTP handlers
│   ├── httpx/              # HTTP helpers
│   ├── panel/              # panel client
│   └── subscription/       # логика subscription headers
├── Dockerfile
├── docker-compose.yml
└── main.go
```

## Важно

- это сервис выдачи подписок, а не VPN-панель
- живые узлы, UUID и production bootstrap-конфиги лучше держать вне репозитория
- если меняешь формат шаблонов, проверь ожидания admin UI и admin API
