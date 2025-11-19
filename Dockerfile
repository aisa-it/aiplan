# syntax=docker/dockerfile:1

# ============================================
# Этап 1: Сборка Frontend
# ============================================
# Build args для альтернативного npm registry (опциональные)
ARG NPM_REGISTRY=""
ARG NPM_AUTH_TOKEN=""
ARG NPM_USERNAME=""
ARG NPM_PASSWORD=""
ARG NPM_EMAIL=""

FROM node:22.18.0 AS frontend-builder

WORKDIR /src

# Сначала копируем package-файлы для лучшего кеширования слоев
COPY aiplan-front/package.json aiplan-front/yarn.lock* ./

# Копируем остальной исходный код
COPY aiplan-front/ ./

# Настройка альтернативного npm registry если указан
ARG NPM_REGISTRY
ARG NPM_AUTH_TOKEN
ARG NPM_USERNAME
ARG NPM_PASSWORD
ARG NPM_EMAIL

RUN if [ -n "$NPM_REGISTRY" ]; then \
      echo "Настройка альтернативного npm registry: $NPM_REGISTRY"; \
      yarn config set registry "$NPM_REGISTRY" --global; \
      REGISTRY_HOST=$(echo "$NPM_REGISTRY" | sed 's|https\?://||' | sed 's|/$||'); \
      if [ -n "$NPM_AUTH_TOKEN" ]; then \
        echo "Использование токена аутентификации для registry"; \
        echo "//$REGISTRY_HOST/:_authToken=$NPM_AUTH_TOKEN" >> /root/.npmrc; \
      elif [ -n "$NPM_USERNAME" ] && [ -n "$NPM_PASSWORD" ]; then \
        echo "Использование username/password для registry"; \
        PASSWORD_BASE64=$(echo -n "$NPM_PASSWORD" | base64); \
        echo "//$REGISTRY_HOST/:username=$NPM_USERNAME" >> /root/.npmrc; \
        echo "//$REGISTRY_HOST/:_password=$PASSWORD_BASE64" >> /root/.npmrc; \
        if [ -n "$NPM_EMAIL" ]; then \
          echo "//$REGISTRY_HOST/:email=$NPM_EMAIL" >> /root/.npmrc; \
        fi; \
      fi; \
    else \
      echo "Использование дефолтного npm registry (registry.npmjs.org)"; \
    fi

# Устанавливаем зависимости и собираем с использованием кеш-монтирования
# Важно: node_modules должны быть установлены в слое, а не только кешироваться
# Кешируем только директории .yarn и .quasar
RUN --mount=type=cache,target=/root/.yarn \
    --mount=type=cache,target=/src/.quasar \
    yarn install --frozen-lockfile && \
    yarn build

# ============================================
# Этап 2: Сборка Backend
# ============================================
FROM golang:alpine AS backend-builder

RUN apk add --no-cache curl

WORKDIR /src

COPY aiplan.go/go.mod aiplan.go/go.sum ./

# Скачиваем зависимости с использованием кеш-монтирования
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Копируем исходный код
COPY aiplan.go/ ./

# Собираем приложение
ARG VERSION=dev
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    GOOS=linux go build \
    -o /build/aiplan \
    -ldflags "-s -w -X main.version=${VERSION}" \
    cmd/aiplan/main.go

# ============================================
# Этап 3: Финальный Runtime-образ
# ============================================
FROM alpine:latest

ENV TZ=Europe/Moscow

RUN apk add --no-cache curl tzdata git openssh-keygen

WORKDIR /app

# Копируем собранный бинарник из backend-сборщика
COPY --from=backend-builder /build/aiplan /app/app

# Копируем справочную документацию
COPY aiplan-help/ /app/aiplan-help/

# Копируем собранный frontend из frontend-сборщика
COPY --from=frontend-builder /src/dist/pwa /app/spa

# Устанавливаем переменную окружения для пути к frontend
ENV FRONT_PATH=/app/spa

EXPOSE 8080 2112

ENTRYPOINT ["/app/app"]

LABEL org.opencontainers.image.source="https://github.com/aisa-it/aiplan"
LABEL org.opencontainers.image.licenses="MPL-2.0"
