# PR Reviewer Assignment Service
*Тестовое задание Backend Internship (осень 2025)*

Сервис автоматически назначает ревьюеров для Pull Request’ов, позволяет управлять командами, пользователями и выполнять операции merge, переназначение и массовую деактивацию.

Полностью соответствует спецификации из `openapi.yaml`.  
Реализованы также все дополнительные задания: статистика, нагрузочные тесты, массовая деактивация, интеграционные тесты, линтер, миграции и docker-compose.


---

#  Структура проекта

```
/cmd/app/main.go
/internal/http/handlers.go
/internal/http/middleware.go
/internal/domain/models.go
/internal/domain/service.go
/internal/repo/postgres.go
/internal/repo/migrations.go
/migrations/*.sql
/tests/e2e/e2e_test.go
/load/k6-pr.js
/.golangci.yml
/Makefile
/Dockerfile
/docker-compose.yml
/README.md
```

---

# Функциональность

### `/team/add`
Создание команды и её участников.

### `/pullRequest/create`
Создание PR и автоматическое назначение до двух активных ревьюверов из команды автора (исключая автора).

### `/pullRequest/reassign`
Переназначение одного ревьювера на случайного активного участника его команды.  
Недоступно, если PR в статусе `MERGED`.

### `/pullRequest/merge`
Идемпотентное закрытие PR.  
После merge изменение ревьюверов запрещено.

### `/users/getReview`
Получение списка PR, где пользователь назначен ревьювером.

### `/users/bulkDeactivate`
Массовая деактивация всех пользователей команды с безопасным переназначением ревьюверов в открытых PR.

### `/stats/assignments`
Статистика по количеству назначений ревьюверов.

---

#  Запуск

## docker-compose

```
docker compose up --build
```

Сервис доступен по адресу:

```
http://localhost:8080
```

Миграции применяются автоматически.

---

#  Тестирование

## E2E-тесты

```
make test
```

### Результат:

```
--- PASS: TestE2E_Flow_CreatePR_Assign_Reassign_Merge
--- PASS: TestE2E_BulkDeactivate_Reassign
PASS
ok   prsrv/tests/e2e 0.848s
```

---

#  Нагрузочные тесты (k6)

### Запуск:

```
BASE_URL=http://localhost:8080 k6 run load/k6-pr.js
```

### Результаты:

| Метрика | Значение |
|--------|----------|
| p95 | 9.81 ms |
| Среднее | 4.3 ms |
| Ошибки сервера | 0% |
| Максимум | 40.78 ms |
| RPS | ~22 req/s |

SLI ≤300ms выполнен с запасом.

---

#  Линтер

Используется **golangci-lint**.  
Конфигурация — `.golangci.yml`.

Запуск:

```
make lint
```

Активные линтеры: errcheck, staticcheck, revive, gofumpt, gocyclo, unused, ineffassign, misspell, bodyclose, sqlclosecheck, govet, gosimple.

---

#  Вывод

Сервис:

- полностью реализует требования тестового задания;
- проходит все E2E-тесты;
- выдерживает нагрузку с большим запасом;
- использует линтер и миграции;
- покрывает все дополнительные задания.
