# Этап 6 — полный набор правил и документация (дизайн)

## Intent и why

`sweep` после этапа 5 — законченный продукт с 3 правилами. Этап 6 делает его
релизом v1: полный встроенный набор правил из `docs/sweep-spec.md` («Встроенные
правила v1», 27 правил) и документация, при которой сообщество сможет добавлять
правила через pull request, не трогая Go-код. Веха: **релиз v1**.

## Goals / Non-goals / Anti-goals

- **Goal:** все 27 правил из таблицы спеки в `rules/`; `docs/rules.md` с
  результатом проверки каждого пути; `docs/format.md`; `CONTRIBUTING.md`;
  скриншот TUI в README.
- **Non-goal:** новые_ln возможности ядра (kind: find, крупные папки — этап 7);
  остановка служб; правило Prefetch (спека прямо запрещает).
- **Anti-goal (что сделает работу провалом):** правило, путь которого доказанно
  неверный, оставшееся «на всякий случай»; проверка «на глаз» без записи
  результата в `docs/rules.md`.

## Constraints

- Windows одной машины: **Windows 10 Pro for Workstations, 22H2, build
  19045**. Проверка путей возможна только здесь.
- Формат правил фиксирован `schema/rule.schema.json` (schema: 1); поля, которые
  понадобятся: `default_enabled`, `requires_closed`, `tags`, `min_age_days`,
  `mode: contents|self|files`, `kind: command` — всё уже существует в ядре.
- Интерфейс — английский, имена и описания правил английские.

## Как было решено (решения пользователя)

- Проверка путей ведётся на этой машине (Win 10 22H2); Windows 11 остаётся
  «not verified» с пометкой в `docs/rules.md`.
- Правило отсутствующего здесь приложения **не удаляется**, если формат пути
  задокументирован вендором; удаляется только доказанно неверный путь
  (отклонение от буквы спеки, ruling). Пометка: `not verifiable here
  (app absent; vendor-documented)`.

## Правила (27 шт.)

Источник истины — таблица спеки `docs/sweep-spec.md`, строки 489–516. Группировка
в файлах по каталогам как сейчас: `rules/browsers/`, `rules/system/`,
`rules/dev/`, `rules/apps/`. Новые ID:

- browsers: `browsers.edge.cache`, `browsers.firefox.cache`
- system: `system.windows-temp`, `system.wer-user`, `system.wer-machine`,
  `system.cbs-logs`, `system.crash-dumps`, `system.minidumps`,
  `system.windows-update-cache`, `system.delivery-optimization`,
  `system.thumbnail-cache`, `system.recycle-bin` (command:
  `powershell.exe -NoProfile -Command Clear-RecycleBin -Force`),
  `system.component-cleanup` (command: `dism.exe /Online /Cleanup-Image
  /StartComponentCleanup`)
- dev: `dev.pnpm-store`, `dev.yarn-cache`, `dev.pip-cache`,
  `dev.go-build-cache`, `dev.nuget-cache`, `dev.gradle-caches`,
  `dev.docker-prune` (command: `docker system prune -f`, dangerous)
- apps: `apps.vscode.cache`, `apps.discord.cache`, `apps.slack.cache`

Риск, admin, default, теги — ровно как в таблице спеки. Просмотр реальных
папок на этой машине (PowerShell) подтверждает пути, существование и то,
что внутри — кэш; результат идёт в `docs/rules.md` для каждого пути.

## Верификация в `docs/rules.md`

Формат: по таблице, одна строка на правило.

| Колонка | Значения |
| --- | --- |
| Проверка Win 10 (22H2, 19045) | `verified` (путь и кэш подтверждены) / `verified: folder absent` (машина чиста — путь валиден по формату, папки нет) / `not verifiable here` (приложение не установлено) / `removed` |
| Windows 11 | `not verified` (для всех) |
| Примечание | что увидели (например «thumbcache держится explorer.exe») |

Замечания о реальном поведении на этой машине (explorer держит
thumbnail-файлы, временные занятые файлы пропускаются) попадают в колонку
примечаний — это тот материал, который спека хочет в rules.md.

## Документация

- `docs/rules.md` — как выше.
- `docs/format.md` — формат правил на примерах: полный разбор одного правила
  (все поля), команды (`kind: command`), маски `%VARS%` и `*`, теги,
  `min_age_days`, `requires_closed`, риск и что он меняет в дефолтах и в TUI;
  отсылка к `schema/rule.schema.json`.
- `CONTRIBUTING.md` — шаблон правила (готовый YAML-каркас) и чек-лист ревью
  PR с правилом (санитария: guard, риск честный, путь проверен на реальной
  машине, описание на английском, одно приложение — один файл).
- `README.md` — скриншот TUI в раздел Status.

## Что уже есть и остаётся на месте

- Embed `rules/` в `internal/rules` (go:embed) — новые YAML попадают автоматически.
- Executor умеет `kind: command` (этап 4), tui рендерит command-правила.
- Тесты правил грузят все файлы набора автоматически — тест «все 27 грузятся,
  уникальные id, схема зелёная» largely бесплатен; усилить явным тестом.

## Left to the builder

- Точные тексты `description` правил (кратко, по-английски, «что построят
  заново и чем пахнет»).
- Порядок и глубинаPowerShell-команд проверки путей ( Puppeteer только чтение
  каталогов, никаких удалений без слова пользователя).
- Разбивка задач плана: правила по категориям, потом проверка и docs.
- Скриншот TUI: как снять (Windows Terminal, шрифт по умолчанию, экран списка
  на синетической фейковой машине не выйдет; снять на реальном скане с
  читаемым текстом).
