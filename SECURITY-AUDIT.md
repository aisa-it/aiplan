# Аудит безопасности AIPlan — вынос во внешний доступ с LDAP

> Дата: 2026-06-10. Цель: вынос приложения из корпоративной сети в публичный интернет с авторизацией через LDAP.
> Метод: 6 параллельных аудиторов по доменам (аутентификация, LDAP, сетевая поверхность, авторизация/IDOR, конфиг/секреты, инъекции/ввод).
> **Вердикт: в текущем виде выносить наружу НЕЛЬЗЯ.** Есть несколько CRITICAL, дающих мгновенный полный захват инстанса из интернета.

---

## ОБНОВЛЕНИЕ под реальную модель развёртывания (2026-06-10)

Уточнённая модель: наружу через ingress (TLS) открыт **только сам AIPlan** (HTTP). LDAP, MinIO, Postgres, Prometheus, SSH — остаются во внутренней сети. Прокси `/<bucket>` дропается на ingress. `SECRET_KEY` задан реальным значением. Суперюзер сменил пароль. `LDAP_FORCE=true`.

### Снято этими мерами (принятый/остаточный риск)
- **C1** — `SECRET_KEY` из env. *(Рекомендация-копейка: добавить fail-fast при пустом/дефолтном — страховка от будущего мисконфига.)*
- **C2** — пароль супера сменён, БД не пуста → `AddDefaultUser` не триггерится.
- **C3 / C7** — дроп `/<bucket>` на ingress + MinIO внутри. Проверено: фронт не использует прямые bucket-URL (раздача через `/api/auth/file/`), дроп безопасен — сделать smoke-test картинок.
- **C6 / H7** — LDAP TLS: MITM-риск снижен (LDAP внутри), downgrade до LOW. Фикс на одну строку всё равно желателен.
- **H10 / H11 / H13 / H14** — Prometheus / БД / Docker-root / SSH не открыты миру.
- **H6** — частично: `LDAP_FORCE=true` отключает локальный fallback. Остаётся: хэш LDAP-пароля всё ещё пишется в БД (`http-authentication.go:323`) → утечка БД = хэши AD.

### Обновление статусов (по итогам ревью с заказчиком)
- **H1 CORS** — ✅ закрыто. CORS-middleware удалён совсем: фронт раздаётся бекендом (embed SPA в проде, Vite-proxy в деве) → весь трафик same-origin, CORS не нужен. Без заголовков браузер режет cross-origin по умолчанию (строже, чем allowlist). Если фронт переедет на отдельный домен — вернуть CORS с явным `AllowOrigins`.
- **H2 security-заголовки** — ✅ добавлены (`http.go:445`, SecureWithConfig: XFrameOptions=DENY, nosniff, HSTS). Хвост: проверить `X-Forwarded-Proto: https` на ingress (иначе HSTS не уйдёт); CSP/ReferrerPolicy — опционально на потом.
- **H4 Content-Type XSS** — ✅ **закрыто.** В `assetsHandler` (`http-minio-redirect.go`) добавлен `Content-Disposition`: inline-allowlist (растровые картинки + application/pdf — рендерятся в песочнице), всё остальное (`text/html`, `image/svg+xml`, office, архивы) → `attachment` с percent-кодированным именем (защита от header injection через имя файла). Скачка на фронте не затронута (blob-based, `a.download`), inline-картинки и PDF-превью работают.
- **Rate-limit логина** — ✅ accepted: общий лимитер `/api` (20rps/IP) + капча с реальным ключом (PoW) + per-account lockout. Остаток: cross-account спрей (душится стоимостью PoW).
- **H15 tus** — ✅ понижено до LOW/accepted: `DisableDownload:true` (нет чтения чужого), POST валидируется, `uploadId` = crypto/rand (нет слепого IDOR). Остаток: при утечке URL — отмена/порча ОДНОЙ незавершённой загрузки (DoS). Опционально: auth на PATCH/DELETE.

### ⚠️ НЕ закрыто сетевой границей — публичная поверхность AIPlan (приоритет!)
Эти находки на самом интернет-facing аппе, забор не помогает:

1. **Брутфорс логина (CRITICAL, усилен `LDAP_FORCE`)** — `http-authentication.go:341,376`: при `LDAP_FORCE=true` весь lockout (`BlockedUntil`/`LoginAttempts`) пропускается. Единственный барьер — капча, ключ которой публичен (**C4**). Итог: неограниченный онлайн-перебор против корпоративного AD + риск DoS-блокировки доменных учёток. Фикс: IP-rate-limit на `/sign-in` независимо от `LDAP_FORCE` + ключ капчи в env.
2. **C5** LDAP fail-open / пустой пароль (`ldap.go:91-98`) — независимо от сети, это сам auth-механизм. Fix обязателен.
3. **H3 / H5** SSRF — **принято как закрытое Kubernetes**: egress пода зарезан до критической инфры (требует default-deny egress + allowlist — подтвердить). Остаточный риск, который NetworkPolicy НЕ покрывает: облачная метадата `169.254.169.254` — link-local pod→node, многие CNI не фильтруют (актуально только если облако; on-prem — неактуально).
   - **`file://` в PDF-экспорте — СНЯТО (не уязвимость).** Подтверждено: санитайзер `UgcPolicy` (bluemonday UGCPolicy) разрешает только схемы `mailto/http/https`, поэтому `file://` из пользовательского контента вырезается на сохранении. В `pdf.go` доходят только системные пути, сгенерированные самим кодом (картинки вместо эмодзи). Трогать нельзя — load-bearing.
   - *Открытый вопрос на будущее:* проверить, санитизируется ли HTML, импортированный из Jira (`jira-html-converter.go`), тем же `UgcPolicy` — если импорт пишет в описание мимо `BeforeSave`, это отдельный канал для непросанитизированного контента.
4. **H1** CORS `*`+credentials, **H4** stored XSS через Content-Type, **H2** security-заголовки (можно на ingress), **H15** tus auth-байпас.
5. **M2/M4/M5/M6/M7** — enumeration, IDOR фильтров, невалидируемая роль, нет лимита размера файла.

### Подтвердить в конфиге
- `WEB_URL=https://...` (иначе Secure-cookie не выставится, `http.go:660`).
- `SIGN_UP_ENABLE=false` (в `release/.env:28` стоит `true`).

### Внутренняя гигиена (не блокер)
- **C8** — `LDAP_BIND_PASSWORD` в Helm ConfigMap → перенести в Secret.
- **H8** — аудит прав на запись атрибута `aiplanadmin` в AD.

---

## CRITICAL — блокеры выноса (захват системы снаружи)

| # | Находка | Файл | Суть риска |
|---|---------|------|-----------|
| C1 | Дефолтный `SECRET_KEY=secretkey`, нет валидации при старте | `.env:25`, `release/.env:25`, `config/config.go:103-133` | Ключ известен (OSS). Любой кует JWT с любым `user_id` → вход под суперюзером. Полный обход auth. |
| C2 | Дефолтный суперюзер с паролем `password123` | `dao/util.go:46`, `cmd/aiplan/main.go:273`, `http.go:1077` | Известные `DEFAULT_EMAIL` + `password123` = суперюзер снаружи, пока не пройден онбординг. |
| C3 | Неаутентифицированный прокси на весь S3-бакет | `http.go:584-591` | `GET /<bucket>/<uuid>` проксирует прямо в MinIO в обход `selectFileWithPermissionCheck`. Скачивание любых вложений/доков/PII без авторизации и без rate-limit. |
| C4 | Хардкоженный HMAC-ключ капчи в публичном репо | `captcha.go:25` | Зная ключ (он в OSS), генерируешь валидные решения капчи пачками → капча бесполезна для брутфорса/регистрации. |
| C5 | LDAP: пустой пароль / fail-open bind = успешный вход | `auth-provider/ldap.go:91-98` | Пустой пароль не отсекается до bind (unauthenticated bind). Хуже: `return true` по умолчанию при ЛЮБОЙ ошибке bind кроме кода 49 (таймаут, сервер недоступен → доступ выдан). Знаешь чужой email → входишь. |
| C6 | LDAP TLS без проверки сертификата (`InsecureSkipVerify: true`) | `auth-provider/ldap.go:46,61,109` | MITM: подмена LDAP-сервера, перехват пароля service-аккаунта (ключ к корп. AD) и паролей всех пользователей. |
| C7 | Анонимный download всего MinIO-бакета | `docker-compose.yaml:59`, `release/docker-compose.yaml` | `mc anonymous set download` — бакет открыт на чтение всем. Дублирует C3 на уровне инфры. |
| C8 | Все секреты в Helm лежат в ConfigMap, а не Secret | `aiplan-helm/templates/configmaps.yaml` | `SECRET_KEY`, `DATABASE_URL`, `LDAP_BIND_PASSWORD`, `AWS_SECRET`, токены — в незашифрованном ConfigMap. Виден всем с `get configmap`. |

---

## HIGH — серьёзные дыры, закрыть до или сразу после выноса

| # | Находка | Файл | Суть риска |
|---|---------|------|-----------|
| H1 | CORS: дефолтный wildcard `*` + `AllowCredentials: true` | `http.go:394-396` | `AllowOrigins` не задан → Echo ставит `*`. Межсайтовая утечка данных; одна правка конфига = полный угон сессий. |
| H2 | Полное отсутствие security-заголовков | `http.go:392`, `middlewares.go:32-48` | Нет HSTS, X-Frame-Options, CSP, X-Content-Type-Options. Clickjacking, MIME-sniffing, усиление XSS. |
| H3 | SSRF в импорте из Jira по пользовательскому URL | `http-import.go:34,58`, `issues-import/jira.go:42` | `jira_url` без валидации → сканирование внутр. сети, `169.254.169.254` (cloud metadata). Доступно любому авторизованному. |
| H4 | Stored XSS: content-type файла берётся от клиента, отдаётся inline | `http-util.go:47`, `http-minio-redirect.go:116` | Заливаешь файл с `Content-Type: text/html` → JS исполняется в origin приложения → кража токенов. (аватары безопасны — переэнкод). |
| H5 | SSRF + LFI при экспорте в PDF (img src из контента) | `export/pdf.go:350-388` | `<img src="file:///...">` или `http://169.254...` в описании задачи → сервер читает локальные файлы / ходит во внутр. сеть. |
| H6 | LDAP-пароль кэшируется в локальную БД + fallback при `LDAP_FORCE=false` | `http-authentication.go:323,355-373` | Хэш доменного пароля оседает в БД AIPlan. При `LDAP_FORCE=false` вход возможен локально в обход актуального состояния AD (отозванный юзер всё ещё входит). |
| H7 | LDAP StartTLS необязателен, fallback на plaintext | `auth-provider/ldap.go:61-64` | Провал StartTLS лишь логируется (Debug), bind идёт в открытую. Downgrade-атака: MITM блокирует StartTLS. |
| H8 | LDAP: эскалация до суперюзера через атрибут `aiplanadmin` | `auth-provider/ldap.go:150-153`, `maintenance/ldap_sync.go:20` | Кто пишет в атрибут каталога — назначает себе админа AIPlan. Sync периодически перетирает локальные права. |
| H9 | `release/.env`: `SIGN_UP_ENABLE=true` + `CAPTCHA_DISABLED=true` | `release/.env:28-29` | Открытая публичная регистрация без капчи во внешнем контуре. Противоречит «только LDAP». |
| H10 | Prometheus `/metrics` (2112) слушает все интерфейсы, без auth | `http.go:608-613`, `docker-compose.yaml:9` | Утечка внутренней телеметрии, помощь в разведке. |
| H11 | Дефолтные креды БД/MinIO + проброс портов наружу | `.env:3-4,16-17`, `docker-compose.yaml:24` | `aiplan/aiplan`, `access-key/secret-key`; порт 5432 (и 9000/9090) проброшен на хост. |
| H12 | TLS не в приложении; Secure-cookie зависит от схемы `WEB_URL` | `http.go:660,673`, `.env:24` | Дефолт `WEB_URL=http://...`. Забыл https → JWT-куки без флага Secure, перехват по открытому каналу. |
| H13 | Docker-контейнер работает от root | `Dockerfile:54-77` | Нет `USER`. Компрометация процесса = root в контейнере. |
| H14 | SSH Git сервер (22222): rate-limit выключен по умолчанию, bind наружу | `ssh-server.go:88-103`, `.env:35` | `SSH_RATE_LIMIT_ENABLED=false` → без лимита. Перебор ключей/DoS если торчит наружу. |
| H15 | tus-загрузки обходят AuthMiddleware для PATCH/GET/DELETE | `http-authentication.go:62-67` | GET/DELETE по `uploadId` без токена → возможен IDOR к чужим незавершённым загрузкам. |

---

## MEDIUM — закрыть в плановом порядке

| # | Находка | Файл |
|---|---------|------|
| M1 | Нет выделенного rate-limit на `/sign-in`/`/forgot-password` (только общий 20rps/IP); при `LDAP_FORCE=true` lockout вообще пропускается | `http.go:430`, `http-authentication.go:341,376` |
| M2 | User enumeration на sign-in (разные ответы для существующего/заблокированного аккаунта) | `http-authentication.go:342-352` |
| M3 | Reset-токен пароля без TTL (бессрочный UUID) | `http-user.go:709-740,962` |
| M4 | IDOR на чтение чужого поискового фильтра (нет проверки owner/public) | `http-user.go:1796`, `api-context.go:439` |
| M5 | Энумерация всех пользователей по email/id любым авторизованным | `http-user.go:153-164` |
| M6 | Смена роли не валидирует значение (можно выставить роль 999) | `http-workspace.go:1936`, `http-project.go:916` |
| M7 | Нет лимита размера загружаемых файлов (DoS по диску) | `http.go:397-415` |
| M8 | refresh-токен живёт 30 дней | `types/const.go:7-8` |
| M9 | MCP-сервер: write-API (create/update issue/doc) без rate-limit | `http.go:554`, `mcp/server.go:137` |
| M10 | Argument injection в git browse (`ref` с ведущим `-`); `repoName` без `ValidateRepositoryName` | `http-git.go:1272,963` |
| M11 | HTML-инъекция в email-уведомлениях о form-ответах (`template.HTML` на пользовательском вводе) | `business/form.go:23-53` |
| M12 | Blacklist токенов in-memory — при мультиподе logout/ротация не видны другим подам | `tokens-cache`, нужен `EXTERNAL_MEMDB` |
| M13 | Анонимная загрузка файлов через публичные формы (`createFormAttachmentsNoAuth`) | `http-form.go:119` |
| M14 | `EMAIL_USE_TLS=false` по умолчанию — SMTP-пароль и письма открытым текстом | `.env:12` |
| M15 | Short-url редиректы как oracle существования issue/doc (без rate-limit) | `http-short-url.go`, `http.go:540` |
| M16 | Swagger UI без auth при `SWAGGER=true` | `http.go:529` |
| M17 | Sync LDAP падает с panic при отсутствии атрибутов (`mail`/`aiplan`) | `auth-provider/ldap.go:150` |

---

## Что СДЕЛАНО ХОРОШО (проверено, дыр нет)

- **SQL-инъекции**: поиск полностью параметризован (`websearch_to_tsquery(?, ?)`), `order_by`/`group_by` по whitelist. Raw-запросы оперируют только системными именами таблиц.
- **JWT alg-confusion**: везде проверяется `*jwt.SigningMethodHMAC` — подмена на `none`/RS256 не проходит.
- **LDAP injection**: ввод экранируется через `ldap.EscapeFilter`.
- **XSS из редактора**: bluemonday UGCPolicy с жёсткими matcher'ами, без `AllowDataURI`/iframe.
- **XXE / zip-slip**: XML не парсится; распаковки пользовательских архивов нет.
- **Path traversal в хранилище**: файлы именуются по UUID.
- **Command injection**: все `exec.Command` без шелла, аргументы массивом.
- **Ротация refresh-токенов** + blacklist + `ResetUserSessions` при смене пароля.
- **forgotPassword** не раскрывает существование аккаунта (возвращает 200).
- **IDOR между workspace/project** на уровне сущностей закрыт guard-middleware (проверка членства ДО загрузки).
- **Маскировка секретов** в стартовых логах и `api-logger` работает.

---

## План действий перед выносом наружу

### Этап 0 — БЛОКЕРЫ (без этого не выносить вообще)
1. `SECRET_KEY` — сгенерировать случайный (≥32 байта), добавить fail-fast при пустом/дефолтном (C1).
2. Убить дефолтный `password123` — генерировать случайный пароль при первом старте + форсить смену (C2).
3. Закрыть прокси `/<bucket>` и анонимный download MinIO; раздавать файлы только через авторизованный `assetsHandler`/presigned-URL (C3, C7).
4. Капча: вынести HMAC-ключ в секрет, убрать дефолт из кода (C4).
5. LDAP: fail-closed bind + отсекать пустой пароль; `InsecureSkipVerify: false` + CA/ServerName; обязательный TLS (`ldaps://`), без plaintext-fallback (C5, C6, H7).
6. Helm: секреты из ConfigMap → Secret (C8).

### Этап 1 — конфигурация внешнего контура (.env / Helm)
- `SIGN_UP_ENABLE=false`, `LDAP_FORCE=true`, `CAPTCHA_DISABLED=false`, `SWAGGER=false`, `MCP_ENABLED=false` (если не нужен).
- `WEB_URL=https://...` (для Secure-cookie), `EMAIL_USE_TLS=true`.
- Сменить дефолтные креды БД и MinIO.
- При `LDAP_FORCE=true` — не кэшировать LDAP-пароль локально (H6).

### Этап 2 — сеть / reverse-proxy / firewall
- TLS терминировать на прокси, наружу — только 8080. Настроить `IPExtractor` для корректного `X-Forwarded-For`.
- **Закрыть наружу**: 2112 (Prometheus), 5432 (Postgres), 9000/9090 (MinIO), 22222 (SSH — если внешний git не нужен).
- Добавить `middleware.Secure` (HSTS, X-Frame-Options, CSP, nosniff) — H2.
- Задать CORS `AllowOrigins: [WEB_URL]`, без `*`+credentials — H1.
- Docker: добавить непривилегированного `USER` — H13.

### Этап 3 — код (плановые правки)
- SSRF-фильтрация (private/loopback/metadata) в Jira-импорте и PDF-экспорте; запрет `file://` (H3, H5).
- `Content-Disposition: attachment` + nosniff + серверное определение типа для загружаемых файлов (H4).
- Выделенный rate-limit на `/sign-in`/`/forgot-password`/`/sign-up` независимо от `LDAP_FORCE` (M1).
- IDOR-фикс фильтров (M4), валидация роли (M6), лимит размера файлов (M7), TTL reset-токена (M3), единый ответ sign-in (M2).
- Аудит прав на запись атрибутов `aiplanadmin`/`aiplan` в каталоге (H8).
- guard'ы на отсутствующие атрибуты в LDAP Sync (M17).
