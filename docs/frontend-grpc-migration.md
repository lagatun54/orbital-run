# Переход браузерного клиента на gRPC-Web

Браузер обращается к публичному адресу `game` по gRPC-Web. Через этот же адрес
он открывает WebSocket на `/ws` и получает кадры матча. Публичный адрес —
origin бэкенда, например `http://localhost:8080` или `https://api.example.com`,
без пути. Для отдельного фронтенда на бэкенде задайте `FRONTEND_ORIGIN` равным
origin страницы, включая порт, если он есть.

## Какие вызовы заменить

| Старый вызов | Новый RPC | Что использовать в клиенте |
| --- | --- | --- |
| `GET /api/leaderboard` | `LeaderboardService.GetLeaderboard` | Вызвать с `{}`, отобразить массив `response.entries`. |
| `GET /api/results/{id}` | `ResultsService.GetResult` | Передать `{ id }`, взять очки из `response.result.score`, а место из `response.rank`. |
| `POST /api/local-results` | `ResultsService.SaveLocalResult` | Передать `{ id, name, score, botScore }`. Успешный RPC возвращает пустое сообщение. |

Контракты версионируются в пакете `orbital.scores.v1`. Клиент строит пути и
сериализует сообщения по двум отдельным определениям:
[LeaderboardService](../api/scores/v1/leaderboard_service.proto) и
[ResultsService](../api/scores/v1/results_service.proto). Общая структура
результата описана в [match_result.proto](../api/scores/v1/match_result.proto).

## Зависимости и сборка

В корне репозитория фронтенда установите зависимости и сохраните обновлённый
`package-lock.json`:

```sh
npm install @bufbuild/protobuf@2.17.0 @connectrpc/connect@2.3.0 @connectrpc/connect-web@2.3.0
```

- **TypeScript/Vite.** Используйте [команду генерации TypeScript](../README.md#генерация-клиентов-и-обработчиков).
  Она создаёт файлы в `src/gen/scores/v1`. Три вызова находятся в
  `src/leaderboard/sync.ts`. Origin для локальной сборки задаёт
  `VITE_BACKEND_URL`.
- **JavaScript с `web/build.mjs`.** Три вызова находятся в `web/game.js`,
  в функциях `refreshLeaderboard`, `waitForResult` и `syncLocalResults`.
  Сгенерируйте JavaScript в `web/gen` следующей командой из корня бэкенда.
  Текущая копирующая сборка не разрешает npm-импорты браузера автоматически.
  Подключите сборщик для `web/game.js`, чтобы он включил Connect и Protobuf
  в `dist/game.js`. Origin по-прежнему берётся из `BACKEND_URL` при сборке.

```sh
npm exec --package=@bufbuild/protoc-gen-es@2.17.0 -- buf generate \
  --template '{"version":"v2","plugins":[{"local":"protoc-gen-es","out":"web/gen","opt":["target=js","import_extension=js"]}]}' \
  --output ../orbital-run-frontend
```

Например, если для JavaScript-версии выбрать esbuild (`npm install -D esbuild`),
импортируйте `build` из `esbuild` в `web/build.mjs`, уберите `game.js` из
списка копируемых файлов и добавьте:

```js
await build({
  entryPoints: ['web/game.js'],
  bundle: true,
  format: 'esm',
  outfile: 'dist/game.js',
  external: ['./config.js', './pixi.min.mjs'],
})
```

`config.js` по-прежнему создаётся скриптом сборки, а `pixi.min.mjs` копируется
в `dist`. HTML продолжает загружать `game.js` как модуль.

## Подключение и обработка результатов

Создайте один транспорт и два клиента, если настроен адрес бэкенда. Пути
импортов в примере подходят для `web/game.js`. В `src/leaderboard/sync.ts`
замените префикс `./gen/` на `../gen/`.

```js
import { createClient } from '@connectrpc/connect'
import { createGrpcWebTransport } from '@connectrpc/connect-web'
import { LeaderboardService } from './gen/scores/v1/leaderboard_service_pb.js'
import { ResultsService } from './gen/scores/v1/results_service_pb.js'

const transport = backendOrigin
  ? createGrpcWebTransport({ baseUrl: backendOrigin })
  : null
const leaderboard = transport && createClient(LeaderboardService, transport)
const results = transport && createClient(ResultsService, transport)
```

Замените вызовы `fetch` в существующих функциях, сохранив обновление интерфейса,
таймауты и привязку к текущему раунду:

- Для загрузки рейтинга вызовите
  `await leaderboard.getLeaderboard({}, { timeoutMs: 4000 })`. Передайте
  `entries` в существующий рендер. Флаг `local` определяет пометку «локально».
- После матча опрашивайте
  `await results.getResult({ id }, { timeoutMs: 3000 })`. `rank` в Protobuf
  имеет тип `int64` и приходит в JS как `bigint`. Если интерфейсу нужен
  `number`, преобразуйте через `Number(rank)` и проверьте, что это
  положительное безопасное целое. Перед обновлением интерфейса проверьте,
  что раунд ещё текущий.
- Для очереди локальных результатов вызовите
  `await results.saveLocalResult({ id, name, score, botScore }, { timeoutMs: 5000 })`.
  Передавайте оба счёта явно, в том числе `0`. Допустимое значение каждого —
  целое число от 0 до 300. Сохраняйте исходный `id` локального матча при
  повторной отправке и удаляйте запись из очереди только после успеха.
  Повторная отправка с тем же ID не меняет сохранённый результат.

Клиент Connect возвращает ошибку через отклонённый промис. При необходимости
проверяйте `ConnectError.from(error).code` и значения `Code` из
`@connectrpc/connect`. Для `GetResult` код `NotFound` сразу после матча
ожидаем: результат ещё может быть в очереди. Сохраните паузу 750 мс между
попытками и общий лимит 12 секунд. `Unavailable` означает, что сервис
временно не может ответить, а `InvalidArgument` от `SaveLocalResult` указывает
на ошибку в полях записи. Если у экрана есть `AbortSignal`, передавайте его
в параметры RPC вместе с таймаутом.

## Проверка перед выпуском

1. Соберите клиент через `npm run build` и откройте его с настроенным адресом
   нового бэкенда. Проверьте, что рейтинг загружается и что в Network виден
   `POST /orbital.scores.v1.LeaderboardService/GetLeaderboard`. При разных
   origin браузер также отправляет preflight `OPTIONS`.
2. Сыграйте онлайн-матч. После завершения должны появиться очки и место
   из `ResultsService.GetResult`.
3. Сыграйте локально при недоступном сервере и восстановите связь. Запись
   должна отправиться через `SaveLocalResult`, появиться с пометкой
   «локально» и удалиться из очереди. Повторная отправка того же ID не должна
   создавать дубль.

Выпускайте изменения клиента и бэкенда вместе: старый клиент обращается к
маршрутам `/api/*`, удалённым в новом бэкенде.
