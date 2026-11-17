# Развёртывание Orbital Run

## Сборка и доставка через GitHub Actions

Процесс [Backend](../.github/workflows/backend.yaml) проверяет Go-код и запускает
сквозные тесты в Compose. После успешной проверки в основной ветке он публикует
образ для Linux AMD64 и ARM64 в `ghcr.io/lagatun54/orbital-run-backend`.
Ссылка на образ с `@sha256:…` появится в **Summary** запуска.

Для постоянной работы бэкенда нужен Linux-хост с Docker Compose 2.24.4 или новее: GitHub
Pages обслуживает статику, GHCR хранит образ, а раннер GitHub Actions работает
только на время задачи. Для доступа из Pages направьте домен API на хост и
откройте 80 и 443 для Caddy. В домашнем каталоге пользователя деплоя создайте
каталог `orbital-backend` с файлом `.env`:

```dotenv
DEMO_DOMAIN=api.example.com
FRONTEND_ORIGIN=https://owner.github.io
POSTGRES_PASSWORD=replace_with_generated_hex
```

`FRONTEND_ORIGIN` — адрес страницы без пути и завершающего слеша. Для
`https://owner.github.io/repo/` это `https://owner.github.io`; для собственного
домена укажите его. Сгенерируйте пароль через `openssl rand -hex 24` и
ограничьте доступ к файлу командой `chmod 600 .env`. Если пакет GHCR приватный,
один раз выполните на хосте `docker login ghcr.io` с токеном с доступом
`read:packages`.

Чтобы выкатывать бэкенд из GitHub:

1. В **Settings → Environments** создайте окружение `backend`, доступное
   только из основной ветки.
2. Задайте в окружении переменные `BACKEND_SSH_HOST` (имя хоста или IPv4,
   SSH-порт 22) и `BACKEND_SSH_USER` (пользователь с доступом к Docker).
   Добавьте секреты `BACKEND_SSH_KEY` (приватный ключ пользователя деплоя) и
   `BACKEND_SSH_KNOWN_HOSTS` (строка `known_hosts` с проверенным ключом хоста).
3. Задайте переменную **репозитория** `DEPLOY_BACKEND=true` и запустите
   **Backend**. Workflow загрузит файлы Compose и Caddy, запустит
   опубликованный образ и дождётся готовности сервисов. Проверьте загрузку
   общего рейтинга в браузерном клиенте с новым адресом бэкенда.

Если нужно запустить бэкенд вручную, скопируйте `compose.yaml`,
`compose.public.yaml` и `Caddyfile` в каталог с `.env`, подставьте ссылку на
образ из **Summary** и выполните:

```sh
BACKEND_IMAGE='ghcr.io/owner/repo-backend@sha256:…' \
  docker compose -f compose.yaml -f compose.public.yaml \
    up -d --no-build --pull always --wait
```

Для отката подставьте предыдущий digest и используйте файлы Compose из
соответствующего выпуска. Данные PostgreSQL и Kafka сохраняются в томах Compose.
После сброса Redis сервис `scores` заполнит кэш при следующем запросе топ-10.

При обновлении с версии, использующей NATS, дайте старому `scores` обработать очередь
JetStream перед обновлением. Уже записанные результаты остаются в PostgreSQL.
Kafka начинает работу со своим топиком и собственной группой потребителей.

## Демо на Northflank с NATS JetStream

В проекте нужны PostgreSQL 17 и два сервиса:

- `orbital-nats` запускает `nats:2.14-alpine` с аргументами
  `-js -sd /tmp/jetstream`. Порт TCP `4222` доступен внутри проекта.
  Без постоянного тома очередь может сброситься при замене контейнера.
- `orbital-run` собирается из корневого `Dockerfile` с target `northflank`.
  В одном контейнере запускаются `game` и `scores`. Откройте только
  HTTP-порт `8080`, чтобы Northflank выдал адрес HTTPS.

В runtime-переменных `orbital-run` задайте `NATS_URL=nats://orbital-nats:4222`
и `FRONTEND_ORIGIN=https://lagatun54.github.io`. Сервисы используют NATS,
когда задан `NATS_URL` и пуст `KAFKA_BROKERS`. Подключите
`POSTGRES_URI` из addon через Secret Group с алиасом `DATABASE_URL`.
Если в проекте есть Redis, передайте его адрес через `REDIS_URL`.
Задайте публичный адрес сервиса в `BACKEND_URL` при следующем запуске
**Frontend** и проверьте загрузку рейтинга в браузере.

Если прямой адрес Northflank недоступен из сети игрока, можно развернуть
[прокси на Render](https://render.com/deploy?repo=https%3A%2F%2Fgithub.com%2Flagatun54%2Forbital-run-backend).
Прокси передаёт gRPC-Web и WebSocket на Northflank. Задайте адрес Render
в `BACKEND_URL`, запустите **Frontend** и проверьте рейтинг из нужной сети.

## Публичный API на одном хосте

Для публичного API нужен Linux-хост с Docker Compose и доменом, указывающим на
его IP. Откройте порты 80 и 443: Caddy настроит HTTPS и проксирование WebSocket.
Создайте `.env` с доменом API, origin отдельно опубликованного клиента и паролем:

```dotenv
DEMO_DOMAIN=api.example.com
FRONTEND_ORIGIN=https://owner.github.io
POSTGRES_PASSWORD=replace_with_generated_hex
```

```sh
docker compose -f compose.yaml -f compose.public.yaml up -d --build --wait
```

Укажите `https://api.example.com` как `BACKEND_URL` при сборке клиента.

### Масштабирование матчей в Compose

В публичном стеке можно указать в `.env` `GAME_REPLICAS=2` и
`GAME_MAX_MATCHES=64`. Это даёт 128 слотов на две реплики. Фактическая ёмкость
зависит от ресурсов хоста. Caddy проверяет список реплик через Docker DNS с
интервалом обновления 5 секунд и направляет новые подключения `/ws` на менее
занятую реплику. Когда слоты заполнены, новые матчи получают `503`. Рейтинг
доступен и при заполненных слотах.

После изменения только `GAME_REPLICAS` добавьте реплики командой ниже. Для стека
из GHCR укажите тот же `BACKEND_IMAGE`, с которым он запущен.

```sh
docker compose -f compose.yaml -f compose.public.yaml up -d --no-deps --no-recreate --wait game
```

Существующий WebSocket остаётся на своей реплике до конца раунда. При уменьшении
числа реплик или пересоздании контейнера матчи на остановленной реплике прервутся.
Изменение `GAME_MAX_MATCHES` применяется при пересоздании `game` и Caddy обычной
командой запуска публичного стека.

Каждый `game` отдаёт на `/metrics` число занятых слотов, их лимит и счётчик
отказов обработчика. Слот учитывается с начала WebSocket-рукопожатия до выхода
из обработчика. Для публичного Compose метрики доступны внутри сети контейнеров,
например с первой реплики:

```sh
docker compose -f compose.yaml -f compose.public.yaml exec -T --index 1 game \
  wget -qO- http://127.0.0.1:8080/metrics
```
