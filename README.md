# nil-loader

`nil-loader` — инструмент нагрузочного тестирования gRPC с real-time дашбордом и автоматическим определением точки деградации.

## О сервисе

`nil-loader` помогает быстро проверить, как gRPC-сервис ведет себя под нагрузкой, и определить безопасный уровень RPS.

## Базовые функции

- нагрузочное тестирование одного gRPC `unary`-метода;
- профили нагрузки `Constant` и `Ramping`;
- Discover через gRPC Reflection или загрузка собственного `.proto`;
- генерация payload по шаблонам (Faker/CSV);
- real-time метрики в UI (`p50/p95/p99`, success rate, ошибки);
- детекция точки деградации и рекомендация безопасного RPS;
- экспорт JSON-отчета.

## Ограничения

- поддерживаются только unary-методы;
- метрики хранятся в памяти и не сохраняются между перезапусками.

## Локальный запуск в Docker

1. Собрать образ:

```bash
make docker-build
```

2. Запустить `nil-loader` и `testservice` вместе:

```bash
make docker-run-all
```

3. Открыть UI:

```text
http://localhost:8080
```

4. Ввести пароль в UI (по умолчанию: `merlion`).

## Локальный запуск из исходников

Запуск сразу основного и тестового сервиса вместе:

```bash
make build
make run-all
```

UI доступен по адресу `http://localhost:8080`.

## Как запустить testservice отдельно

```bash
make run-test
```

Для проверки в UI укажите:

- `Target Host`: `localhost:50051`

Дальше используйте Discover (Reflection), чтобы выбрать сервис и метод.

## Полезные ссылки

- Презентация: [`internal/docs/nil-loader_full.pdf`](internal/docs/nil-loader_full.pdf)
- Сервис в интернете: [`https://nil-loader.ru/`](https://nil-loader.ru/)