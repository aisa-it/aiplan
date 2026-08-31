#!/usr/bin/env bash
# Запускает workflow_dispatch сборку docker-образа на ветке dev и печатает
# тег собранного образа.
#
# Использование:
#   scripts/dev-build.sh                 # запустить и дождаться, вывести тег
#   scripts/dev-build.sh --branch main   # другая ветка
#   scripts/dev-build.sh --json          # машиночитаемый вывод
#   scripts/dev-build.sh --attach 12345  # не запускать новую, дождаться уже идущей
#   scripts/dev-build.sh --no-notify     # без уведомления в macOS
#
# На macOS по окончании шлёт уведомление: terminal-notifier → osascript → bell.
# Если Центр уведомлений режет terminal-notifier ("Could not request notification
# permission"), положи terminal-notifier.app в /Applications и разреши его в
# System Settings → Notifications, либо укажи путь: TERMINAL_NOTIFIER=/путь/к/бинарю.
#
# Совместим с bash 3.2 (системный bash в macOS): без here-strings (<<<),
# без (( )) и без внешнего jq — разбор JSON отдан gh --jq.
#
# stdout — только результат (тег/JSON), весь прогресс идёт в stderr,
# поэтому безопасно писать: IMAGE=$(scripts/dev-build.sh)

set -eu

WORKFLOW="docker-push.yml"
JOB_NAME="build-and-push"
IMAGE_NAME="ghcr.io/aisa-it/aiplan"
BRANCH="dev"
TIMEOUT=3600
INTERVAL=15
OUTPUT="plain"
ATTACH_RUN=""
REPO="${REPO:-}"
NOTIFY=1
RUN_URL=""
RUN_ID=""

log() { printf '>> %s\n' "$*" >&2; }

# macOS-уведомление. Каскад из трёх попыток, каждая может тихо отвалиться:
#   1) terminal-notifier — сначала копия в /Applications (только у неё обычно
#      есть разрешение Центра уведомлений), потом любой из PATH;
#   2) osascript — права спрашиваются у хост-приложения (Terminal/iTerm),
#      которое чаще всего уже разрешено;
#   3) звуковой bell — последний рубеж, работает всегда.
# На Linux/без всего этого — тихий no-op, скрипт никогда не роняется.
NOTIFIER_APP="/Applications/terminal-notifier.app/Contents/MacOS/terminal-notifier"

notifier_bin() {
  if [ -x "${TERMINAL_NOTIFIER:-}" ]; then
    printf '%s\n' "$TERMINAL_NOTIFIER"
  elif [ -x "$NOTIFIER_APP" ]; then
    printf '%s\n' "$NOTIFIER_APP"
  elif command -v terminal-notifier >/dev/null 2>&1; then
    command -v terminal-notifier
  fi
}

notify() {
  notify_title="$1"
  notify_message="$2"
  notify_sound="${3:-Glass}"
  [ "$NOTIFY" = "1" ] || return 0

  notify_bin=$(notifier_bin)
  if [ -n "$notify_bin" ]; then
    # stderr ловим: при запрете уведомлений terminal-notifier пишет
    # "Could not request notification permission", но выходит с кодом 0.
    if [ -n "$RUN_URL" ]; then
      notify_err=$("$notify_bin" -title "$notify_title" -message "$notify_message" \
        -sound "$notify_sound" \
        -group "aiplan-dev-build" -open "$RUN_URL" 2>&1 >/dev/null) || notify_err="failed"
    else
      notify_err=$("$notify_bin" -title "$notify_title" -message "$notify_message" \
        -sound "$notify_sound" \
        -group "aiplan-dev-build" 2>&1 >/dev/null) || notify_err="failed"
    fi
    [ -z "$notify_err" ] && return 0
    log "terminal-notifier не смог ($notify_err), пробую osascript"
  fi

  if command -v osascript >/dev/null 2>&1; then
    notify_body=$(printf '%s' "$notify_message" | sed 's/["\\]/\\&/g')
    notify_head=$(printf '%s' "$notify_title" | sed 's/["\\]/\\&/g')
    if osascript -e "display notification \"$notify_body\" with title \"$notify_head\" sound name \"$notify_sound\"" >/dev/null 2>&1; then
      return 0
    fi
    log "osascript тоже не пустили в Центр уведомлений"
  fi

  printf '\a' >&2
  return 0
}

die() {
  printf '!! %s\n' "$*" >&2
  notify "AIPlan: сборка не удалась" "$*" Basso
  exit 1
}

usage() {
  awk 'NR>1 && /^#/ { sub(/^# ?/, ""); print; next } NR>1 { exit }' "$0" >&2
  exit "${1:-0}"
}

while [ $# -gt 0 ]; do
  case "$1" in
    -b|--branch)   BRANCH="$2"; shift 2 ;;
    -w|--workflow) WORKFLOW="$2"; shift 2 ;;
    -r|--repo)     REPO="$2"; shift 2 ;;
    -t|--timeout)  TIMEOUT="$2"; shift 2 ;;
    -i|--interval) INTERVAL="$2"; shift 2 ;;
    --attach)      ATTACH_RUN="$2"; shift 2 ;;
    --json)        OUTPUT="json"; shift ;;
    --no-notify)   NOTIFY=0; shift ;;
    -h|--help)     usage 0 ;;
    *) die "неизвестный аргумент: $1 (--help в помощь)" ;;
  esac
done

command -v gh >/dev/null 2>&1 || die "gh не найден в PATH. Ставь GitHub CLI: https://cli.github.com"
gh auth status >/dev/null 2>&1 || die "gh не авторизован. Выполни: gh auth login"

if [ -z "$REPO" ]; then
  REPO=$(gh repo view --json nameWithOwner -q .nameWithOwner) || REPO=""
  [ -n "$REPO" ] || die "не смог определить репозиторий, задай --repo owner/name"
fi
log "репозиторий: $REPO, ветка: $BRANCH, workflow: $WORKFLOW"

# --- 1. Запуск -------------------------------------------------------------
# workflow_dispatch через API не возвращает id созданного run, поэтому
# запоминаем максимальный id ДО запуска и ждём появления большего.
if [ -n "$ATTACH_RUN" ]; then
  RUN_ID="$ATTACH_RUN"
  log "подключаюсь к существующему run #$RUN_ID"
else
  BASELINE=$(gh run list --repo "$REPO" --workflow "$WORKFLOW" --branch "$BRANCH" --limit 1 --json databaseId -q '.[0].databaseId // 0') || BASELINE=0
  [ -n "$BASELINE" ] || BASELINE=0
  log "последний известный run: $BASELINE"

  gh workflow run "$WORKFLOW" --repo "$REPO" --ref "$BRANCH" || die "не удалось запустить workflow (нет прав на dispatch или ветка без workflow-файла?)"
  log "workflow запущен, ищу свежий run..."

  attempt=0
  while [ "$attempt" -lt 30 ]; do
    attempt=$((attempt + 1))
    sleep 2
    RUN_ID=$(gh run list --repo "$REPO" --workflow "$WORKFLOW" --branch "$BRANCH" --event workflow_dispatch --limit 10 --json databaseId -q "[.[] | select(.databaseId > $BASELINE)] | max_by(.databaseId) | .databaseId // empty") || RUN_ID=""
    [ -n "$RUN_ID" ] && break
  done
  [ -n "$RUN_ID" ] || die "новый run так и не появился за 60с, глянь: gh run list --workflow $WORKFLOW"
fi

RUN_URL="https://github.com/$REPO/actions/runs/$RUN_ID"
log "run #$RUN_ID → $RUN_URL"

# --- 2. Ожидание нужного job ----------------------------------------------
# Ждём только build-and-push: соседний generate-api к образу отношения не имеет
# и может идти дольше или падать. Данные тянем одной строкой TSV — так не нужен
# внешний jq, хватает встроенного в gh --jq.
JOB_ID=""
JOB_CONCLUSION=""
JOB_QUERY='.jobs[] | select(.name == "'"$JOB_NAME"'") | [.id, .status, (.conclusion // "")] | @tsv'
NOW=$(date +%s)
DEADLINE=$((NOW + TIMEOUT))

while : ; do
  JOB_LINE=$(gh api "repos/$REPO/actions/runs/$RUN_ID/jobs" --jq "$JOB_QUERY" 2>/dev/null) || JOB_LINE=""

  if [ -n "$JOB_LINE" ]; then
    JOB_ID=$(printf '%s\n' "$JOB_LINE" | cut -f1)
    JOB_STATUS=$(printf '%s\n' "$JOB_LINE" | cut -f2)
    JOB_CONCLUSION=$(printf '%s\n' "$JOB_LINE" | cut -f3)
    log "job $JOB_NAME (#$JOB_ID): $JOB_STATUS${JOB_CONCLUSION:+ / $JOB_CONCLUSION}"
    [ "$JOB_STATUS" = "completed" ] && break
  else
    log "job $JOB_NAME ещё не создан..."
  fi

  NOW=$(date +%s)
  [ "$NOW" -lt "$DEADLINE" ] || die "таймаут ${TIMEOUT}с. Сборка ещё идёт: $RUN_URL"
  sleep "$INTERVAL"
done

[ "$JOB_CONCLUSION" = "success" ] || die "сборка завершилась с результатом '$JOB_CONCLUSION': $RUN_URL"

# --- 3. Достаём версию из лога --------------------------------------------
# Шаг "Resolve version and channel" печатает: Building <version>, moving tag: <tag>
LOGS=$(gh api "repos/$REPO/actions/jobs/$JOB_ID/logs" 2>/dev/null) || LOGS=""
if [ -z "$LOGS" ]; then
  LOGS=$(gh run view "$RUN_ID" --repo "$REPO" --job "$JOB_ID" --log 2>/dev/null) || LOGS=""
fi

BUILD_LINE=$(printf '%s\n' "$LOGS" | grep -oE 'Building v[0-9A-Za-z.+-]+, moving tag: [a-z]+' | tail -n 1) || BUILD_LINE=""
VERSION=""
MOVING=""
if [ -n "$BUILD_LINE" ]; then
  VERSION=$(printf '%s\n' "$BUILD_LINE" | sed -E 's/^Building (v[^,]+), moving tag: .*/\1/')
  MOVING=$(printf '%s\n' "$BUILD_LINE" | sed -E 's/.*moving tag: //')
fi

if [ -z "$VERSION" ]; then
  # Запасной путь: та же формула, что в workflow — v<major>.<minor+1>.0-dev.<run_number>.g<sha7>
  log "версию из лога вытащить не вышло, считаю по формуле workflow"
  RUN_META=$(gh api "repos/$REPO/actions/runs/$RUN_ID" --jq '[.run_number, .head_sha] | @tsv') || die "не смог получить метаданные run #$RUN_ID"
  RUN_NUMBER=$(printf '%s\n' "$RUN_META" | cut -f1)
  SHA7=$(printf '%s\n' "$RUN_META" | cut -f2 | cut -c1-7)
  BASE=$(git tag --sort=-v:refname 2>/dev/null | head -n 1) || BASE=""
  [ -n "$BASE" ] || BASE="v0.0.0"
  BASE="${BASE#v}"
  MAJOR=$(printf '%s\n' "$BASE" | cut -d. -f1)
  MINOR=$(printf '%s\n' "$BASE" | cut -d. -f2)
  [ -n "$MINOR" ] || MINOR=0
  VERSION="v${MAJOR}.$((10#$MINOR + 1)).0-dev.${RUN_NUMBER}.g${SHA7}"
  MOVING="dev"
  log "внимание: версия вычислена локально, сверь локальные теги (git fetch --tags)"
fi

# --- 4. Вывод --------------------------------------------------------------
if [ "$OUTPUT" = "json" ]; then
  printf '{\n'
  printf '  "image": "%s:%s",\n'       "$IMAGE_NAME" "$VERSION"
  printf '  "moving_tag": "%s:%s",\n'  "$IMAGE_NAME" "$MOVING"
  printf '  "version": "%s",\n'        "$VERSION"
  printf '  "branch": "%s",\n'         "$BRANCH"
  printf '  "run_id": "%s",\n'         "$RUN_ID"
  printf '  "run_url": "%s"\n'         "$RUN_URL"
  printf '}\n'
else
  log "готово: также обновлён плавающий тег $IMAGE_NAME:$MOVING"
  printf '%s:%s\n' "$IMAGE_NAME" "$VERSION"
fi

notify "AIPlan: образ собран" "$IMAGE_NAME:$VERSION (moving: $MOVING)"
