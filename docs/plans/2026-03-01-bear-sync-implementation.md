# Plan: bear-sync — полная реализация

## Overview

Реализация bear-sync: центральный хаб (Go API на VPS) + Mac-агент (bear-bridge) для синхронизации заметок Bear с openclaw. Чтение + запись + вложения + конфликты. Архитектурный дизайн: `docs/plans/2026-02-27-bear-sync-design.md`.

Монорепо с двумя бинарниками (`cmd/hub`, `cmd/bridge`). Стек: Go + chi + SQLite (modernc.org/sqlite). Паттерны и тулинг из citeck-ci.

**Явно исключено из этого плана (отложено):**
- `DELETE /api/notes/:id/tags/:tag_id` (remove_tag) — Bear x-callback-url не имеет эндпоинта для удаления тега. Workaround через body rewrite рискует повредить inline-теги. Требуется исследование.
- Bear reset reconciliation (`POST /api/sync/reconcile`) — до реализации Bear reset требует ручного вмешательства.
- Исследование альтернатив xcall (Swift CLI через NSWorkspace).

## Validation Commands

- `make lint`
- `make test`
- `make build`

### Task 1: Скаффолдинг проекта

- [x] Создать `go.mod` (module `github.com/romancha/bear-sync`, Go 1.24)
- [x] Создать `Makefile` с таргетами: `build`, `build-local`, `test`, `test-coverage`, `test-race`, `lint`, `fmt`, `tidy`, `clean`, `generate`. Build собирает оба бинарника (`cmd/hub`, `cmd/bridge`)
- [x] Создать `.golangci.yml` (25 линтеров: gosec, govet, staticcheck, wrapcheck, errorlint, shadow, line limit 140 — по образцу citeck-ci)
- [x] Создать структуру директорий: `cmd/hub/`, `cmd/bridge/`, `internal/models/`, `internal/mapper/`, `internal/beardb/`, `internal/hubclient/`, `internal/store/`, `internal/api/`, `internal/xcallback/`, `testdata/`
- [x] Создать `CLAUDE.md` с инструкциями для AI-ассистента (паттерны кода, структура, команды)
- [x] Добавить зависимости в go.mod: `github.com/go-chi/chi/v5`, `modernc.org/sqlite`, `github.com/stretchr/testify`, `github.com/matryer/moq`

### Task 2: Общие модели (`internal/models`)

- [x] `note.go` — структура `Note` со всеми полями из схемы БД (dual-id: `ID` + `BearID`, все флаги, даты ISO 8601, `SyncStatus`, `HubModifiedAt`, `BearRaw`). JSON-теги для API.
- [x] `tag.go` — структура `Tag` (dual-id, все поля из схемы, `BearRaw`)
- [x] `attachment.go` — структура `Attachment` (dual-id, `Type` file/image/video, медиа-метаданные, `FilePath`, `BearRaw`)
- [x] `backlink.go` — структура `Backlink` (dual-id, `LinkedByID`, `LinkingToID`, `BearRaw`)
- [x] `write_queue.go` — структура `WriteQueueItem` (`ID`, `IdempotencyKey`, `Action` create/update/add_tag/trash, `NoteID`, `Payload` JSON, `Status`, `ProcessingBy`, `LeaseUntil`, `AppliedAt`, `Error`)
- [x] `sync.go` — структуры `SyncPushRequest` (notes, tags, note_tags, pinned_note_tags, attachments, backlinks, deleted_*_ids), `SyncAckRequest`, `SyncAckItem` (поля: `QueueID`, `IdempotencyKey`, `Status` applied/failed, `BearID` — UUID из Bear для заполнения notes.bear_id при ack, `Error`)

### Task 3: Маппер Bear → Hub (`internal/mapper`)

- [x] `mapper.go` — функции маппинга: `MapBearNote(row) → Note`, `MapBearTag(row) → Tag`, `MapBearAttachment(row) → Attachment`, `MapBearBacklink(row) → Backlink`
- [x] Конвертация Core Data epoch → ISO 8601 (`+978307200`, `time.Unix()`, `Format(time.RFC3339)`)
- [x] Конвертация `Z_ENT` → тип: 8→`file`, 9→`image`, 10→`video`
- [x] Формирование `bear_raw` JSON из оригинальных полей Bear
- [x] Генерация hub UUID: `hex(randomblob(16))` аналог через `crypto/rand`
- [x] Unit-тесты маппера: конвертация дат (включая NULL), маппинг всех полей, формирование bear_raw, edge cases (NULL bear_id, зашифрованные заметки)

### Task 4: SQLite store хаба (`internal/store`)

- [x] `store.go` — интерфейс `Store` со всеми методами (CRUD notes/tags/attachments/backlinks, FTS5 search, write_queue, sync_meta, sync/push)
- [x] `sqlite.go` — реализация `SQLiteStore`: инициализация БД, `PRAGMA journal_mode=WAL`, `PRAGMA busy_timeout=5000`, `PRAGMA foreign_keys=ON`
- [x] Миграция схемы: создание всех таблиц (notes, tags, note_tags, pinned_note_tags, attachments, backlinks, write_queue, sync_meta), индексов, FTS5 virtual table + триггеры
- [x] CRUD Notes: `ListNotes` (фильтры tag/trashed/encrypted, пагинация limit/offset, сортировка), `GetNote` (с тегами и бэклинками), `CreateNote`, `UpdateNote`
- [x] FTS5 поиск: `SearchNotes(query, tag, limit)` — поиск по title+body, опциональный фильтр по тегу
- [x] CRUD Tags: `ListTags`, `GetTag`, `CreateTag`
- [x] CRUD Attachments: `GetAttachment`, `ListAttachmentsByNote`
- [x] CRUD Backlinks: `ListBacklinksByNote`
- [x] Sync Push: `ProcessSyncPush(req SyncPushRequest)` — upsert notes/tags/attachments/backlinks по bear_id, DELETE+INSERT note_tags/pinned_note_tags (scope: note_id IN push), обработка deleted_*_ids. При `sync_status=pending_to_bear`: НЕ перезаписывать body/title
- [x] Write Queue: `EnqueueWrite(idempotencyKey, action, noteID, payload)` — idempotency check, `LeaseQueueItems(processingBy, leaseDuration)` — lease-based выдача (default lease 5 мин), `AckQueueItems(items []SyncAckItem)` — идемпотентный ack по idempotency_key (повторный ack — no-op), заполнение `notes.bear_id` из ack при create, expire stale leases (processing + lease_until < now → pending)
- [x] Sync Meta: `GetSyncMeta(key)`, `SetSyncMeta(key, value)`. Ключи: `last_sync_at`, `last_push_at`, `queue_size`, `bear_db_hash`, `initial_sync_complete`
- [x] Unit-тесты store: все CRUD операции, FTS5 поиск, upsert idempotency, write_queue lifecycle (enqueue → lease → ack), FK cascade delete, sync_push с pending_to_bear, lease expiry

### Task 5: HTTP API хаба (`internal/api`)

- [x] `server.go` — chi роутер, middleware (auth Bearer token, `http.MaxBytesReader` с лимитами: 50 MB для `sync/push`, 100 MB для `sync/attachments/:id`, 1 MB для остальных openclaw API, request logging через slog, recovery)
- [x] Auth middleware: `Authorization: Bearer <token>`, два токена (openclaw + bridge), разные права (bridge: sync/*, openclaw: api/*)
- [x] `notes_handler.go` — `GET /api/notes` (список без body, с тегами), `GET /api/notes/search?q=...` (FTS5, static route — chi приоритизирует static над параметризованными автоматически), `GET /api/notes/:id` (полная заметка + теги + backlinks), `POST /api/notes` (create → write_queue), `PUT /api/notes/:id` (update → write_queue), `DELETE /api/notes/:id` (trash → write_queue)
- [x] `tags_handler.go` — `GET /api/tags` (все теги с иерархией), `POST /api/notes/:id/tags` (add_tag → write_queue)
- [x] `backlinks_handler.go` — `GET /api/notes/:id/backlinks` (заметки, ссылающиеся на данную)
- [x] `attachments_handler.go` — `GET /api/attachments/:id` (скачать файл с диска)
- [x] `sync_handler.go` — `POST /api/sync/push`, `GET /api/sync/queue` (lease-based), `POST /api/sync/ack` (идемпотентный), `POST /api/sync/attachments/:id` (upload файла), `GET /api/sync/status`
- [x] Idempotency: обязательный `Idempotency-Key` header для POST/PUT/DELETE openclaw API. Повторный запрос с тем же ключом → существующий результат
- [x] Запрет записи зашифрованных заметок: 403 Forbidden для PUT/DELETE на notes с `encrypted=1`
- [x] Unit-тесты API: все эндпоинты через `httptest`, auth (valid/invalid/wrong scope), валидация, пагинация, фильтры, idempotency, 403 на encrypted

### Task 6: Hub main (`cmd/hub`)

- [ ] `main.go` — чтение конфигурации из env (`HUB_PORT`, `HUB_DB_PATH`, `HUB_OPENCLAW_TOKEN`, `HUB_BRIDGE_TOKEN`, `HUB_ATTACHMENTS_DIR`)
- [ ] Инициализация SQLiteStore, API server
- [ ] Graceful shutdown: перехват `SIGTERM`/`SIGINT` → `context.WithCancel` → `http.Server.Shutdown(ctx)` таймаут 10 сек → закрытие SQLite
- [ ] Структурированное логирование через `slog` (JSON в production)
- [ ] Bind на `127.0.0.1:PORT` (только localhost, Caddy проксирует)

### Task 7: Чтение Bear SQLite (`internal/beardb`)

- [ ] `beardb.go` — интерфейс `BearDB` со всеми методами чтения
- [ ] `sqlite.go` — реализация: открытие Bear БД read-only (`?mode=ro`), путь `~/Library/Group Containers/9K33E3U3T4.net.shinyfrog.bear/Application Data/database.sqlite`
- [ ] Чтение `ZSFNOTE` — все колонки, delta по `ZMODIFICATIONDATE >= lastSyncAt`
- [ ] Чтение `ZSFNOTETAG` — все колонки, delta по `ZMODIFICATIONDATE`
- [ ] Чтение `ZSFNOTEFILE` — все колонки, delta по `ZMODIFICATIONDATE`
- [ ] Чтение `ZSFNOTEBACKLINK` — все колонки (включая `ZLINKEDBY`, `ZLINKINGTO` — resolve Z_PK → UUID через JOIN с ZSFNOTE), delta по `ZMODIFICATIONDATE`
- [ ] Чтение junction tables: `Z_5TAGS` (Z_5NOTES, Z_13TAGS) и `Z_5PINNEDINTAGS` (Z_5PINNEDNOTES, Z_13PINNEDINTAGS) — full read, resolve Z_PK → UUID через JOIN
- [ ] Чтение полного списка UUID всех сущностей (для обнаружения удалений)
- [ ] Unit-тесты: парсинг Bear SQLite из `testdata/` (тестовая БД с заметками, тегами, вложениями, бэклинками, зашифрованной заметкой, junction table связями)

### Task 8: HTTP-клиент хаба (`internal/hubclient`)

- [ ] `client.go` — интерфейс `HubClient` и реализация HTTP-клиента для API хаба
- [ ] `SyncPush(ctx, req SyncPushRequest)` — `POST /api/sync/push` с Bearer token
- [ ] `LeaseQueue(ctx, processingBy)` — `GET /api/sync/queue` → список `WriteQueueItem`
- [ ] `AckQueue(ctx, items)` — `POST /api/sync/ack`
- [ ] `UploadAttachment(ctx, attachmentID, reader)` — `POST /api/sync/attachments/:id`
- [ ] `GetSyncStatus(ctx)` — `GET /api/sync/status`
- [ ] Retry с exponential backoff для transient errors
- [ ] Unit-тесты: с `httptest.NewServer` mock-сервером

### Task 9: X-callback-url executor (`internal/xcallback`)

- [ ] `xcallback.go` — интерфейс `XCallback` и реализация через xcall CLI
- [ ] `Create(ctx, token, title, body, tags)` → `bear_id` (UUID из x-success ответа). URL: `bear://x-callback-url/create?token=...&title=...&text=...&tags=...`
- [ ] `Update(ctx, token, bearID, body)` — URL: `bear://x-callback-url/add-text?token=...&id=<bear_id>&text=...&mode=replace`
- [ ] `AddTag(ctx, token, bearID, tag)` — URL: `bear://x-callback-url/add-tag?token=...&id=<bear_id>&tag=...`
- [ ] `Trash(ctx, token, bearID)` — URL: `bear://x-callback-url/trash?token=...&id=<bear_id>`
- [ ] URL-формирование с правильным URL-encoding параметров
- [ ] Маскировка token в логах: `token=***`
- [ ] Проверка наличия xcall при инициализации
- [ ] Unit-тесты: формирование URL, экранирование параметров, маскировка token

### Task 10: Bridge main — экспорт дельт (`cmd/bridge`)

- [ ] `main.go` — точка входа, чтение конфига из env (`BRIDGE_HUB_URL`, `BRIDGE_HUB_TOKEN`, `BEAR_TOKEN`, `BRIDGE_STATE_PATH`)
- [ ] Защита от параллельного запуска: `flock(2)` через `syscall.Flock` на `~/.bear-bridge.lock` (LOCK_EX|LOCK_NB, авто-снятие при crash)
- [ ] State management: чтение/запись `~/.bear-bridge-state.json` (`last_sync_at`, `known_note_ids`, `known_tag_ids`, `known_attachment_ids`, `known_backlink_ids`, `known_note_tag_pairs`, `known_pinned_note_tag_pairs`, `junction_full_scan_counter`)
- [ ] Delta export: чтение изменённых сущностей из Bear SQLite по `ZMODIFICATIONDATE >= last_sync_at` (оператор `>=`, не `>` — overlap-window для защиты от потери записей на границе timestamp при clock drift; дубли безопасны — хаб делает upsert по bear_id), маппинг через mapper, формирование `SyncPushRequest`
- [ ] Junction tables delta: для каждой изменённой заметки — полный snapshot тегов (note_tags + pinned_note_tags)
- [ ] Junction tables full-scan: каждый 12-й цикл — полное чтение, сравнение с saved snapshot, push diff
- [ ] Обнаружение удалений: сравнение текущих UUID с `known_*_ids`, формирование `deleted_*_ids`
- [ ] Initial sync: если нет state-файла → батчевый push всех заметок по 50 штук. После завершения — записать `initial_sync_complete=true` в `sync_meta` хаба через push
- [ ] Push дельты через `hubclient.SyncPush()`
- [ ] Обновление state-файла после успешного push

### Task 11: Bridge main — применение очереди записи

- [ ] Lease очереди: `hubclient.LeaseQueue()` → получение pending items
- [ ] Duplicate-safe apply: перед выполнением проверка в Bear SQLite — уже создано/обновлено/тег добавлен? Если да → ack `applied` без xcall
- [ ] Apply `create`: `xcallback.Create(token, title, body, tags)` → получить bear_id из x-success ответа. Fallback верификация (если xcall не вернул UUID): ждёт 2 сек, ищет в Bear SQLite по `title + ZCREATIONDATE > now-5sec`. Если >1 результат → `failed`
- [ ] Apply `update`: `xcallback.Update(token, bearID, body)`. Верификация: ждёт 2 сек, перечитывает Bear SQLite по bear_id, проверяет что body обновлён
- [ ] Apply `add_tag`: `xcallback.AddTag(token, bearID, tag)`. Верификация: ждёт 2 сек, перечитывает Bear SQLite, проверяет наличие тега
- [ ] Apply `trash`: `xcallback.Trash(token, bearID)` — вызов `bear://x-callback-url/trash?token=...&id=<bear_id>`. Верификация: перечитать Bear SQLite, проверить `ZTRASHED=1`
- [ ] Ack: `hubclient.AckQueue(items []SyncAckItem)` — каждый item содержит `QueueID`, `IdempotencyKey`, `Status` (applied/failed), `BearID` (UUID из Bear — хаб заполнит notes.bear_id), `Error` (описание при failed)
- [ ] Обработка ошибок: failed items логируются, не блокируют остальные

### Task 12: Синхронизация вложений

- [ ] Bridge: после sync push — проверка новых/изменённых attachments, чтение файлов из Bear (`~/Library/Group Containers/.../Local Files/Note Images/<UUID>/` и `Note Files/<UUID>/`)
- [ ] Upload: `hubclient.UploadAttachment(attachmentID, fileReader)` → `POST /api/sync/attachments/:id`
- [ ] Hub: сохранение файлов на диск VPS в `HUB_ATTACHMENTS_DIR/<attachment_id>/<filename>`
- [ ] Hub: `GET /api/attachments/:id` — отдача файлов с диска
- [ ] Очистка: при `permanently_deleted=1` в sync push — удаление файлов с диска VPS

### Task 13: Обработка конфликтов

- [ ] Hub: при sync/push — детекция конфликта: заметка имеет `sync_status=pending_to_bear` (изменена openclaw) И пришёл push с новым `modified_at` из Bear (изменена пользователем). Сравнение: `hub_modified_at` заметки vs `last_push_at` из `sync_meta`. Пометка `sync_status: conflict`
- [ ] Bridge: при lease queue — пропуск items для заметок с `sync_status: conflict`
- [ ] Bridge: создание conflict-заметки `"[Conflict] Original Title"` через xcall с версией openclaw
- [ ] Hub: `GET /api/sync/status` — включение информации о конфликтах

### Task 14: Тестовые фикстуры и E2E тесты

- [ ] Создать тестовую Bear SQLite в `testdata/` (Core Data формат: несколько заметок, теги с иерархией, вложения, бэклинки, зашифрованная заметка, junction table связи)
- [ ] E2E тест чтения: bridge читает testdata Bear SQLite → push на hub (in-process, `:0` порт) → openclaw API читает → проверка данных
- [ ] E2E тест записи: openclaw POST/PUT через API → write_queue → bridge lease → mock xcall ack (с bear_id) → проверка статусов и заполнения notes.bear_id
- [ ] E2E тест idempotency: повторный HTTP запрос с тем же Idempotency-Key → тот же результат, без дубля в write_queue
- [ ] E2E тест crash-recovery: apply без ack → lease expiry (processing + lease_until < now → pending) → повторный pickup → no duplicate
- [ ] E2E тест junction table full-scan: связи тегов изменились без изменения ZMODIFICATIONDATE заметки → delta-цикл не подхватывает → full-scan цикл подхватывает
- [ ] E2E тест encrypted: попытка PUT/DELETE зашифрованной заметки → 403
- [ ] E2E тест конфликтов: openclaw обновляет заметку (pending_to_bear) → bridge push с новым modified_at из Bear → sync_status=conflict → bridge создаёт conflict-заметку

### Task 15: Деплой и конфигурация

- [ ] Systemd unit для hub (`bear-sync-hub.service`): Environment file, restart on failure, after=network.target
- [ ] Launchd plist для bridge (`com.romancha.bear-bridge.plist`): StartInterval 300 (5 мин), env variables
- [ ] Caddyfile: reverse proxy с TLS (Let's Encrypt), rate limiting (60 req/min openclaw, 120 req/min bridge)
- [ ] `.gitignore` — бинарники, .db файлы, state файлы, coverage отчёты
- [ ] Документация в README: установка, конфигурация env-переменных, запуск

### Task 16: Финальная сверка с дизайн-документом

- [ ] Прочитать `docs/plans/2026-02-27-bear-sync-design.md` целиком и сверить с реализацией
- [ ] Проверить схему БД: все таблицы (notes, tags, note_tags, pinned_note_tags, attachments, backlinks, write_queue, sync_meta), все колонки, индексы, FTS5 триггеры соответствуют дизайну
- [ ] Проверить маппинг полей: все поля из таблиц маппинга (ZSFNOTE→notes, ZSFNOTETAG→tags, Z_5TAGS→note_tags, Z_5PINNEDINTAGS→pinned_note_tags, ZSFNOTEFILE→attachments, ZSFNOTEBACKLINK→backlinks) реализованы корректно
- [ ] Проверить API эндпоинты: все эндпоинты из дизайна (openclaw API + sync API) реализованы с правильными контрактами, фильтрами, пагинацией
- [ ] Проверить потоки данных: чтение (Bear→hub→openclaw), запись (openclaw→hub→write_queue→bridge→Bear→ack), initial sync, dual-id lifecycle (hub UUID + bear_id заполняется при ack)
- [ ] Проверить sync/push логику: upsert по bear_id, DELETE+INSERT note_tags/pinned_note_tags (scope: note_id IN push), обработка deleted_*_ids, защита pending_to_bear заметок (body/title НЕ перезаписываются)
- [ ] Проверить write_queue семантику: effectively-once (openclaw→hub через Idempotency-Key), at-least-once (hub→bridge через lease), duplicate-safe apply (bridge проверяет состояние в Bear перед xcall)
- [ ] Проверить ограничения из дизайна: лимит x-callback-url (~50KB body → failed), зашифрованные заметки read-only (403), bridge flock protection
- [ ] Исправить любые расхождения между реализацией и дизайн-документом
