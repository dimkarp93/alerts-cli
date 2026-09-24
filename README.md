# alerts-cli

Маленькая CLI-тулза, чтобы посмотреть, что конкретно сработало за день и что было замьючено.

```
alerts-cli alerts [flags]   срабатывания алертов (история vmalert в VictoriaMetrics)
alerts-cli mutes  [flags]   включения мьютов (сайленсы Alertmanager)
```

## Сборка

```bash
cd ~/tools/alerts-cli && make
```

Ещё цели: `make check` (gofmt + go vet + go test), `make install` (в `$PREFIX/bin`, по умолчанию
`~/.local/bin`), `make clean`. Код лежит в `cmd/alerts-cli/`, версия — в `versions.txt`.

Версии и релизы: `make bump-patch` / `bump-minor` / `bump-major` поднимает версию, коммитит
`versions.txt`, ставит тег `vX.Y.Z` и пушит; тег запускает `.github/workflows/release.yml`,
который собирает архивы под linux/darwin × amd64/arm64 и `SHA256SUMS`. Установка релиза —
`github_install.sh dimkarp93/alerts-cli`, из рабочей копии — `local_install.sh .`.

`alerts-cli --version`, `--origin`, `--buildinfo` печатают версию и происхождение сборки.

Зависимостей нет — только стандартная библиотека Go.

## Откуда данные

- **VictoriaMetrics** — vmalert через `remoteWrite` пишет туда серию `ALERTS`; по ней
  восстанавливаются эпизоды срабатывания (начало, длительность, метки).
- **Alertmanager** — сайленсы (`/api/v2/silences`). Срабатывание считается замьюченным, если
  пересекается по времени с сайленсом, все матчеры которого подходят под его метки. Истёкшие
  сайленсы Alertmanager хранит ограниченное время, поэтому для давних дней мьюты могут быть уже
  недоступны.

## Конфиг

Адреса VictoriaMetrics и Alertmanager не зашиты в программу — берутся только из
`~/.config/alerts-cli/config.json`:

```json
{
  "vm": "http://vm.example.internal:8428",
  "am": "http://am.example.internal:9093"
}
```

Без файла или без одного из полей `alerts-cli` завершается с ошибкой в stderr. Для отдельного
контура (например, test) держите отдельный конфиг и меняйте симлинк
`~/.config/alerts-cli/config.json` на нужный файл перед запуском.

## Даты (одинаково в обеих подкомандах)

| `-date` | значение |
|---|---|
| `2026-09-14` | один день |
| `2026-09-12,2026-09-15` | набор дней |
| `2026-09-10-2026-09-14` | непрерывный диапазон |
| `today` / `yesterday` / `-3` | относительно сегодня |

Границы суток и вывод — в `-tz` (по умолчанию `Europe/Moscow`).

## Вывод для человека

Все форматы, кроме `json`, печатаются таблицей с рамкой и заголовками колонок. Колонка, пустая
во всех строках, не печатается (например, колонки мьюта при `-mode unmuted`), а колонка,
вынесенная в заголовок секции (`▌ ...`) при группировке, не дублируется в строках.

`-col-width 48` — предел ширины колонок «КОММЕНТАРИЙ», «МЕТКИ» и «МАТЧЕРЫ»; длиннее —
обрезается с `…`. `-col-width 0` — не обрезать. Полные значения всегда есть в `-format json`.

## alerts

```bash
alerts-cli alerts                                            # сегодня, хронология
alerts-cli alerts -date yesterday -format stats              # только статистика
alerts-cli alerts -date 2026-09-10-2026-09-14 -mode unmuted -severity critical
alerts-cli alerts -date 2026-09-12,2026-09-15 -format by-group -group-by domain
alerts-cli alerts -format by-name -selector 'alertname=~"Payments.*"'
alerts-cli alerts -date yesterday -format json > alerts.json
```

- `-mode all|unmuted|muted` — все сработавшие / только не замьюченные / только замьюченные.
- `-format chrono|by-name|by-group|stats|json` — хронология / по точному имени алерта /
  по группе (внутри — хронологически) / только счётчики / JSON.
- `-group-by group|component|team|domain|severity` — ключ группировки для `by-group` и `stats`.
- `-severity warning,critical` — фильтр по уровню; без флага показываются все уровни.
  Уровень (`WARN` / `CRIT`) печатается в выводе всегда.
- Прочее: `-state firing|pending|any`, `-env`, `-selector` (доп. PromQL-матчеры), `-step`,
  `-labels`, `-no-silences`, `-col-width`, `-json-compact`.

Колонки: `ВРЕМЯ`, `УР.`, `АЛЕРТ`, `ГРУППА`, `ДЛИТ.`, `МЬЮТ С`, `МЬЮТ ДО`, `КЕМ`,
`КОММЕНТАРИЙ`, `МЕТКИ`. `МЬЮТ С` / `МЬЮТ ДО` — когда мьют был выставлен и до какого момента
установлен; если срабатывание накрыто несколькими сайленсами, показан первый, а в `КЕМ`
добавлено `+N`.

Группировки: `group` — rule-группа vmalert (`mvpay_payments_quality`, `postgresql_cluster`, …),
самая точная; `domain` — крупно `product` (группы `mvpay_*`) против `infrastructure`;
`component` / `team` — одноимённые метки (у части правил пустые, тогда группа `-`); `severity`.

В `stats` для диапазона дней добавляется колонка `ПО ДНЯМ` с разбивкой по дням.

## mutes

```bash
alerts-cli mutes -date 2026-09-10-2026-09-15                   # всё, что действовало в эти дни
alerts-cli mutes -date 2026-09-15 -filter created              # только включённые в этот день
alerts-cli mutes -date 2026-09-15 -filter expired              # только истёкшие в этот день
alerts-cli mutes -date 2026-09-15 -severity critical
alerts-cli mutes -date 2026-09-10-2026-09-15 -format by-name   # по алертам
alerts-cli mutes -date 2026-09-10-2026-09-15 -format by-group -group-by creator
```

Колонки: `ВКЛЮЧЁН` (когда мьют выставлен), `ДО` (до какого момента установлен), `ДЛИТ.`,
`СТАТУС` (`active` / `expired` / `pending`), `УР.`, `АЛЕРТ`, `ГРУППА`, **`КЕМ` (кто включил)**,
`КОММЕНТАРИЙ`, `МАТЧЕРЫ`.

- `-format chrono|by-name|by-group|json`, `-group-by group|severity|creator|domain`.
- `-severity` — тот же фильтр по уровню. Уровень, имя алерта и группа берутся из матчеров
  сайленса, а если их там нет — доопределяются по сериям `ALERTS` за последние 30 дней.
- `-filter all|created|expired` — какие мьюты показывать: по умолчанию `all` — все, что были
  активны в выбранные дни (включая включённые раньше); `created` — только включённые в эти дни;
  `expired` — только те, что истекли в эти дни.
- `-by <автор>` — фильтр по автору (подстрока, регистронезависимо).

## JSON

`-format json` есть в обеих подкомандах: в `alerts` — мета, `episodes[]` и агрегаты
`by_alert[]` / `by_group[]`; в `mutes` — мета и `mutes[]` (`created_by`, `starts_at`/`ends_at`,
`duration_seconds`, `state`, `alerts`, `groups`, `severities`, `domain`, `matchers`).
В `alerts` у замьюченных срабатываний рядом с `MUTED` печатается окно мьюта
(`by=<кто> MM-DD HH:MM→MM-DD HH:MM`), в JSON это `silences[].starts_at` / `ends_at`.
`-json-compact` — в одну строку. Данные всегда идут в stdout, диагностика — в stderr,
так что stdout остаётся валидным JSON.
