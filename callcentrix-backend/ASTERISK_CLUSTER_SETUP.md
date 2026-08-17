# Настройка кластера Asterisk (несколько серверов, одна БД)

Это одноразовая настройка **на каждом физическом сервере Asterisk**, которую
нужно применить руками — у backend нет доступа к файлам конфигурации
Asterisk. Приложение управляет только строками в Postgres (realtime) и шлёт
AMI-команды `pjsip reload`/`dialplan reload`; сама возможность серверам
находить друг друга и звонить между собой обеспечивается конфигом Asterisk
ниже.

Предполагается, что realtime для `endpoint`/`aor`/`auth`/`identify` (таблицы
`ast_ps_*`) и dialplan (`ast_extensions`) уже настроены и работают на текущем
единственном сервере — так уже устроен этот проект. Ниже — что нужно
**добавить**, чтобы то же самое расширилось на несколько серверов.

## 1. Разделяемые живые регистрации (`ast_ps_contacts`)

Это ключевой шаг. Сейчас Asterisk хранит живые SIP-регистрации (`Contact`)
только у себя в памяти. Чтобы сервер A мог напрямую позвонить пользователю,
зарегистрированному на сервере B, регистрации должны храниться в общей
Postgres-базе — тогда `Dial(PJSIP/username)` на любом боксе увидит актуальный
контакт, где бы пользователь ни был подключён, и отправит INVITE прямо туда.
Существующее правило дайлплана `_X. → Dial(PJSIP/${EXTEN},30,rU)` (создаётся
в `CreateTenantContext`) менять не нужно — оно уже работает правильно.

В `sorcery.conf` (обычно `/etc/asterisk/sorcery.conf`) на **каждом** сервере,
рядом с уже существующими секциями для `endpoint`/`aor`/`auth`/`identify`,
добавить:

```ini
[res_pjsip]
contact=realtime,ps_contacts
```

Таблица `ast_ps_contacts` — стандартная PJSIP realtime-схема Asterisk. Если
её ещё нет в базе, создать:

```sql
CREATE TABLE IF NOT EXISTS ast_ps_contacts (
    id                        VARCHAR(255) PRIMARY KEY,
    uri                       VARCHAR(511),
    expiration_time           BIGINT,
    qualify_frequency         INTEGER,
    outbound_proxy            VARCHAR(255),
    path                      TEXT,
    user_agent                VARCHAR(255),
    qualify_timeout           DECIMAL(5,3),
    reg_server                VARCHAR(255),
    authenticate_qualify      CHAR(1),
    via_addr                  VARCHAR(255),
    via_port                  INTEGER,
    call_id                   VARCHAR(255),
    endpoint                  VARCHAR(255),
    prune_on_boot             CHAR(1)
);
```

Это единственная таблица из данного списка, в которую пишет сам Asterisk (не
backend) — в неё ничего провижинить из Go не нужно.

## 2. Порядок идентификации входящих запросов

В `pjsip.conf`, секция `[global]`, убедиться, что:

```ini
[global]
endpoint_identifier_order=ip,username,anonymous
```

Это нужно, чтобы входящий INVITE от другого бокса кластера распознавался по
IP через объект `identify`, который backend создаёт автоматически при
добавлении сервера в админке (см. `internal/asterisk/server.go`,
`writeInterServerTrust`) — по одному `identify` на каждую упорядоченную пару
серверов, с общим контекстом `asterisk-cluster-relay`.

## 3. Контекст `asterisk-cluster-relay` в dialplan realtime

Как и для контекстов провайдеров/тенантов, dialplan realtime подключается
через `switch`, а не читается напрямую из файла — это уже должно быть
настроено в `extensions.conf`:

```ini
[asterisk-cluster-relay]
switch => Realtime/@extensions
```

Backend сам пишет строки этого контекста в `ast_extensions` (одно и то же
правило `_X. → Dial(PJSIP/${EXTEN},30,rU)`, что и в тенантских контекстах) —
руками ничего в контекст добавлять не нужно, только объявить сам контекст в
`extensions.conf`, как уже сделано для `[tenant-N]` и контекстов провайдеров.

## 4. Qualify / NAT

Если серверы находятся в разных сетях (не в одном приватном сегменте),
задать `qualify_frequency`, `rtp_symmetric`, `force_rport` так же, как это
уже сделано для агентских эндпоинтов (`internal/asterisk/sip.go`), и
убедиться, что порты RTP/SIP между серверами открыты в фаерволе в обе
стороны — иначе аудио между звонящими на разных боксах не пройдёт даже при
успешном сигналинге.

## 5. Применение

```
asterisk -rx "core reload"
```

(маппинг sorcery не подхватывается одним `pjsip reload`/`dialplan reload` —
нужен полный `core reload` после правки `sorcery.conf`). После этого шага
всё остальное — добавление серверов, пользователей — идёт через обычный флоу
приложения (`pjsip reload`/`dialplan reload` по AMI, которые backend уже
рассылает на все настроенные серверы при любом изменении).

## Проверка после настройки

1. В админке (SuperAdmin → Тенанты → вкладка «Серверы Asterisk») добавить
   два тестовых сервера.
2. Создать двух пользователей — при 2 активных серверах они должны
   автоматически распределиться по разным боксам (см. колонку «Сервер» на
   странице Пользователи).
3. Убедиться, что оба веб-телефона регистрируются (каждый должен подключаться
   к своему серверу — WS-прокси backend выбирает URI автоматически).
4. Позвонить с одного номера на другой — звонок должен пройти через
   `asterisk-cluster-relay` на удалённом боксе и установиться с двусторонним
   звуком.
