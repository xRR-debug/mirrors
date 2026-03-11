# CS2 Mirrors — Исправленная версия

Инструмент для создания зеркал (фейковых серверов) CS2-CSGO в Steam Game Server Browser на основе [Fess23/Mirrors](https://github.com/Fess23/Mirrors).

## Требования

- Go 1.22+
- Токены игровых серверов Steam (из https://steamcommunity.com/dev/managegameservers, AppID = 730)

## Файлы конфигурации

### tokens.txt
Каждый токен с новой строки. Каждый токен = одно зеркало.
```
XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX
YYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYY
```

### config.json
```json
{
  "start_port": 27016,
  "csgo_mod": false,
  "servers": [
    {
      "count": 10,
      "server_address": "1.2.3.4:27015",
      "players": 1,
      "max_players": 32,
      "bots": 0,
      "hostname": "My Server",
      "map": "de_dust2",
      "region": "3",
      "secure": true,
      "tags": "",
      "use_abuse": false,
      "description": ""
    }
  ]
}
```

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

## Запуск

```bash

# Запуск (Windows)
mirrors.exe

# Запуск (Linux)
./mirrors

# С кастомными путями
./mirrors -config config.json -accounts accounts.txt -tokens tokens.txt
```

# Changelog

Все изменения относительно оригинального репозитория [Fess23/Mirrors](https://github.com/Fess23/Mirrors).

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

### 2. Импорты в protocol/gamecoordinator
**Файлы:** `protocol/gamecoordinator/msg.go`, `protocol/gamecoordinator/packet.go`

Файлы импортировали `github.com/WirStaff/go-steam/protocol` напрямую вместо `wirstaff.com/mirrors/protocol`, что вызывало ошибку сборки после удаления внешней зависимости.

---

## Новый функционал

### 3. Параллельность: 

Добавлен семафор (5 одновременных подключений) для избежания rate-limit от Steam при массовом запуске.

### 4. Логирование: 

Улучшены сообщения для диагностики. Введите любой текст в консоли для отображения количества активных зеркал и игроков.

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

### 13. Поддержка кириллицы в конфиге
**Файл:** `main.go`

Добавлена автоматическая конвертация Windows-1251 → UTF-8 при чтении `config.json`. Если файл сохранён в Windows-1251 (стандартная кодировка Windows) — программа автоматически конвертирует его. Если уже UTF-8 — оставляет как есть.

Теперь можно использовать кириллицу в `hostname`, `map`, `tags`, `description` без дополнительных настроек.

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