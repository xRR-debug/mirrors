# Changelog

Все изменения относительно оригинального репозитория [WirStaff/Mirrors](https://github.com/WirStaff/Mirrors).

---

## Критические баги

### 1. Event loop — зеркала не регистрировались в Steam
**Файл:** `main.go`

В оригинале `break` внутри `switch` прерывал только `switch`, но не внешний `for range`. Из-за этого после получения `ConnectedEvent` цикл немедленно выходил через следующий `break` внизу, не дожидаясь `LoggedOnEvent`. Зеркала никогда не завершали логин.

```go
// Было (сломано):
for event := range s.Events() {
    switch e := event.(type) {
    case *steam.ConnectedEvent:
        s.Logon(token)
        continue  // продолжает цикл — OK
    case *server.LoggedOnEvent:
        // ...
        break  // прерывает switch, НЕ for — баг
    }
    break  // всегда выполняется после первого события
}

// Стало (исправлено):
// используется goto doneLogin для выхода из цикла
```

### 2. Версия сервера
**Файл:** `server/server.go`

Версия по умолчанию была заглушкой `"9.99.9.9"`. Steam отображал зеркала с неверной версией и мог скрывать их из листинга.

```go
// Было:
version: "9.99.9.9"

// Стало:
version: "13992"  // актуальный билд CS2
```

### 3. CGO / sqlite3 — зависание на Linux
**Файл:** `utils/utils.go`, `go.mod`

Импорт `_ "github.com/mattn/go-sqlite3"` требовал CGO и вызывал зависание при компиляции без CGO или на Linux без нужных системных библиотек. Пакет нигде не использовался.

- Удалён импорт `go-sqlite3` из `utils/utils.go`
- Удалены зависимости `github.com/WirStaff/go-steam` и `github.com/mattn/go-sqlite3` из `go.mod`
- Очищен `go.sum` — оставлены только 4 реально используемых пакета
- Исправлен `vendor/modules.txt` (убрана запись о `go-steam`, вызывавшая `inconsistent vendoring`)

### 4. Импорты в protocol/gamecoordinator
**Файлы:** `protocol/gamecoordinator/msg.go`, `protocol/gamecoordinator/packet.go`

Файлы импортировали `github.com/WirStaff/go-steam/protocol` напрямую вместо `wirstaff.com/mirrors/protocol`, что вызывало ошибку сборки после удаления внешней зависимости.

---

## Новый функционал

### 5. A2S UDP сервер (новый пакет `a2s/`)
**Файл:** `a2s/a2s.go` — новый файл

В оригинале не было никакого A2S ответчика. Без него Steam Server Browser не мог получить информацию о сервере при клике — поля "Игра", "Название", "Карта", "Задержка" оставались пустыми, кнопки "Обновить" и "Присоединиться" не работали.

Добавлен полноценный A2S UDP сервер:
- **A2S_INFO** (`0x54`) — двухэтапный challenge протокол (CS2 требует challenge перед INFO)
- **A2S_PLAYER** (`0x55`) — возвращает пустой список игроков
- **A2S_RULES** (`0x56`) — возвращает пустой список правил
- **Challenge** (`0x57`) — генерирует случайный challenge, хранит сессии по IP

#### Двухэтапный challenge (обязателен для CS2):
```
Клиент → TSource Engine Query (25 байт, без challenge)
Сервер → 0xFF 0xFF 0xFF 0xFF 0x41 + 4 байта challenge
Клиент → TSource Engine Query + те же 4 байта (29 байт)
Сервер → 0xFF 0xFF 0xFF 0xFF 0x49 + полный INFO ответ
```

#### Правильный EDF в A2S_INFO ответе:
```
0xB0 = 0x80 (port) | 0x10 (SteamID) | 0x20 (tags)
```
Поля передаются в строгом порядке: port (uint16 LE) → SteamID (uint64 LE) → tags (string).

### 6. UDP прокси на реальный сервер
**Файл:** `a2s/a2s.go`

A2S запросы обрабатываются локально зеркалом. Все остальные UDP пакеты (игровой трафик, подключение игроков) проксируются на `server_address` из конфига. Это позволяет подключаться к зеркалу как к обычному серверу — реальное соединение идёт на основной сервер.

### 7. Передача SteamID в A2S ответ
**Файлы:** `server/listeners.go`, `server/server.go`, `a2s/a2s.go`

После логина Steam присваивает зеркалу SteamID. Этот SteamID теперь сохраняется и передаётся в A2S_INFO ответ через EDF поле `0x10`. Без реального SteamID CS2 Browser считал ответ невалидным и не отображал информацию.

---

## Исправления A2S протокола

### 8. Folder поле — `cs2` вместо `csgo`
**Файл:** `server/server.go`, `a2s/a2s.go`

CS2 Browser фильтрует серверы по полю `Folder` в A2S_INFO. Если `Folder = "csgo"` — сервер не отображается в браузере CS2. Исправлено: `Folder` всегда `"cs2"`.

### 9. EDF флаги — исправлен порядок и значения
Оригинально стояло `EDF = 0x20` (только tags). Исправлено на `0xB0`:
- `0x80` — порт сервера (uint16)
- `0x10` — SteamID (uint64)  
- `0x20` — теги (строка)

---

## Конфиг

### 10. Новые поля в config.json
Добавлены опциональные поля:

```json
{
  "version": "1.41.3.8",
  "game_dir": "csgo"
}
```

- `version` — версия сервера для Steam API (если не указана, используется `13992`)
- `game_dir` — gamedir для регистрации в Steam (если не указан, `cs2` при `csgo_mod: false`, `csgo` при `csgo_mod: true`)

---

## Сборка

### Кросс-компиляция Windows → Linux

```bat
go mod vendor
build_linux.bat
```

Или вручную:
```bat
set GOOS=linux
set GOARCH=amd64
set CGO_ENABLED=0
go build -mod=vendor -a -o mirrors .
```

### На сервере

```bash
pkill -9 mirrors
./mirrors
```

---

## Обновления (v0.1.2)

### 11. Рандомные поля конфига
**Файл:** `main.go`

Поля `hostname`, `map`, `tags`, `description` теперь поддерживают как строку так и массив строк. При запуске каждое зеркало выбирает случайное значение независимо:

```json
"hostname": ["сервер #1", "сервер #2", "сервер #3"],
"map": ["de_dust2", "de_mirage", "de_inferno"]
```

### 12. use_abuse — фейковый онлайн
**Файлы:** `server/server.go`, `main.go`

При `use_abuse: true` сервер отображает количество игроков равное полю `players` — как в Steam API так и в A2S ответе. Заполняется массив фейковых SteamID уникальных для каждого зеркала.

```json
"players": 20,
"max_players": 32,
"use_abuse": true
```

Будет показывать `20/32` в браузере серверов.
