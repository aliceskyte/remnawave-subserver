# Deploy

Этот архив содержит текущее состояние `subserver`, включая:

- исходники и служебные файлы проекта;
- `docker-compose.yml`, `Dockerfile`, `install.sh`.

## Требования

- Linux-сервер с Docker и Docker Compose (`docker compose`);
- подготовленный env-файл с секретами вне каталога проекта;
- отдельный bootstrap-конфиг вне каталога проекта;
- архитектура сервера `amd64`/`x86_64` для сборки текущего Docker-образа без изменений.

## Быстрый запуск на новом сервере

1. Распаковать архив, например в `/opt/subserver`.
2. Перейти в каталог проекта:

```bash
cd /opt/subserver
```

3. Создать отдельный env-файл вне дерева проекта:

```bash
install -d -m 700 /etc/subserver
cp .env.example /etc/subserver/subserver.env
chmod 600 /etc/subserver/subserver.env
```

Заполнить в `/etc/subserver/subserver.env` как минимум `PANEL_URL`, `PANEL_TOKEN` и `ADMIN_TOKEN`.

4. Подготовить bootstrap-конфиг вне дерева проекта:

```bash
install -m 600 configs/default.json /etc/subserver/default.json
```

5. Подготовить каталог для runtime-данных:

```bash
install -d -m 700 /var/lib/subserver
```

6. Создать docker-сеть, если её ещё нет:

```bash
docker network inspect remnawave-network >/dev/null 2>&1 || docker network create remnawave-network
```

7. Поднять сервис:

```bash
export SUBSERVER_CONFIG_FILE=/etc/subserver/default.json
docker compose up -d --build
```

Альтернатива: использовать включённый скрипт:

```bash
./install.sh
```

## Проверка

```bash
docker compose ps
docker compose logs -f subserver
```

По текущему `docker-compose.yml` сервис публикуется только на loopback `127.0.0.1:18080`, admin UI:

```text
http://127.0.0.1:18080/admin/
```

## Что переносится

- секреты не должны лежать в архиве или каталоге проекта;
- bootstrap-конфиг с живыми узлами не должен лежать в публичном git-репозитории или образе;
- боевой env-файл по умолчанию читается из `/etc/subserver/subserver.env`;
- runtime-данные по умолчанию читаются из `/var/lib/subserver`;
- bootstrap-конфиг можно переопределить через `SUBSERVER_CONFIG_FILE=/custom/default.json`;
- при необходимости путь можно переопределить через `SUBSERVER_DATA_DIR=/custom/data-dir`.

## Замечания

- Хранить секреты отдельно от архива и каталога проекта.
- Использовать длинный случайный `ADMIN_TOKEN`, отдельный от токена панели.
- При необходимости можно переопределить путь env-файла через `SUBSERVER_ENV_FILE=/custom/path.env docker compose up -d`.
- Локальный `.env` больше не подхватывается в production; для dev-режима нужен `SUBSERVER_LOAD_DOTENV=1`.
- Не хранить SQLite-базу и runtime-логи внутри каталога проекта.
- Если на новом сервере уже существует каталог с прежней версией `subserver`, перед распаковкой лучше сделать его резервную копию.
