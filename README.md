# CS2 Mirrors — Исправленная версия

Инструмент для создания зеркал (фейковых серверов) CS2 в Steam Game Server Browser.

## Требования

- Go 1.22+
- Токены игровых серверов Steam (из https://steamcommunity.com/dev/managegameservers, AppID = 730)
- Steam аккаунты с CS2 (для фейковых игроков, если нужны)

## Файлы конфигурации

### tokens.txt
Каждый токен с новой строки. Каждый токен = одно зеркало.
```
XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX
YYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYY
```

### accounts.txt
Логин:пароль с новой строки. Используются для фейковых игроков.
```
login1:password1
login2:password2
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

**Регионы:**
- `0` — США восток
- `1` — США запад  
- `2` — Южная Америка
- `3` — Европа
- `4` — Азия
- `5` — Австралия
- `6` — Ближний Восток
- `7` — Африка

## Запуск

```bash
# Сборка
go build -o mirrors .

# Запуск (Windows)
mirrors.exe

# Запуск (Linux)
./mirrors

# С кастомными путями
./mirrors -config config.json -accounts accounts.txt -tokens tokens.txt
```

## Что было исправлено

1. **Главная ошибка**: В оригинальном `main.go` цикл событий сервера и игроков использовал `break` внутри `switch`, что приводило к немедленному выходу из цикла `for range events` после первого же события. Это означало, что после `ConnectedEvent` программа никогда не дожидалась `LoggedOnEvent`, и зеркала не регистрировались в Steam.

2. **Версия сервера**: Изменена с заглушки `9.99.9.9` на актуальный билд CS2 `13992`.

3. **Gamedir**: В CS2 режиме `Gamedir` теперь правильно устанавливается в `cs2` (было жёстко закодировано `csgo`).

4. **Параллельность**: Добавлен семафор (5 одновременных подключений) для избежания rate-limit от Steam при массовом запуске.

5. **Логирование**: Улучшены сообщения для диагностики.

6. **Статус**: Введите любой текст в консоли для отображения количества активных зеркал и игроков.
