package main

import (
	"bufio"
	"encoding/json"
	"math/rand"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"sync"
	"time"
	"wirstaff.com/mirrors/player"
	"wirstaff.com/mirrors/server"
	"wirstaff.com/mirrors/steam"
	"wirstaff.com/mirrors/utils"
)

type Config struct {
	StartPort uint32    `json:"start_port"`
	Servers   []Servers `json:"servers"`
	CSGOMod   bool      `json:"csgo_mod"`
}

// RandomField умеет читать как строку так и массив строк из JSON
type RandomField []string

func (f *RandomField) UnmarshalJSON(data []byte) error {
	// Попробуем как массив
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		*f = arr
		return nil
	}
	// Попробуем как строку
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*f = []string{s}
		return nil
	}
	return fmt.Errorf("cannot unmarshal field")
}

func (f RandomField) Pick() string {
	if len(f) == 0 {
		return ""
	}
	return f[rand.Intn(len(f))]
}

type Servers struct {
	Count         uint32      `json:"count"`
	ServerAddress string      `json:"server_address"`
	Players       uint8       `json:"players"`
	MaxPlayers    uint8       `json:"max_players"`
	Bots          uint8       `json:"bots"`
	Hostname      RandomField `json:"hostname"`
	Map           RandomField `json:"map"`
	Region        string      `json:"region"`
	Secure        bool        `json:"secure"`
	Tags          RandomField `json:"tags"`
	Description   RandomField `json:"description"`
	UseAbuse      bool        `json:"use_abuse"`
	Version       string      `json:"version"`
	GameDir       string      `json:"game_dir"`
}

// win1251toUTF8 конвертирует Windows-1251 строку в UTF-8
func win1251toUTF8(b []byte) []byte {
	// Таблица символов Windows-1251 (0x80-0xFF)
	table := []rune{
		0x0402,0x0403,0x201A,0x0453,0x201E,0x2026,0x2020,0x2021,
		0x20AC,0x2030,0x0409,0x2039,0x040A,0x040C,0x040B,0x040F,
		0x0452,0x2018,0x2019,0x201C,0x201D,0x2022,0x2013,0x2014,
		0xFFFD,0x2122,0x0459,0x203A,0x045A,0x045C,0x045B,0x045F,
		0x00A0,0x040E,0x045E,0x0408,0x00A4,0x0490,0x00A6,0x00A7,
		0x0401,0x00A9,0x0404,0x00AB,0x00AC,0x00AD,0x00AE,0x0407,
		0x00B0,0x00B1,0x0406,0x0456,0x0491,0x00B5,0x00B6,0x00B7,
		0x0451,0x2116,0x0454,0x00BB,0x0458,0x0405,0x0455,0x0457,
		0x0410,0x0411,0x0412,0x0413,0x0414,0x0415,0x0416,0x0417,
		0x0418,0x0419,0x041A,0x041B,0x041C,0x041D,0x041E,0x041F,
		0x0420,0x0421,0x0422,0x0423,0x0424,0x0425,0x0426,0x0427,
		0x0428,0x0429,0x042A,0x042B,0x042C,0x042D,0x042E,0x042F,
		0x0430,0x0431,0x0432,0x0433,0x0434,0x0435,0x0436,0x0437,
		0x0438,0x0439,0x043A,0x043B,0x043C,0x043D,0x043E,0x043F,
		0x0440,0x0441,0x0442,0x0443,0x0444,0x0445,0x0446,0x0447,
		0x0448,0x0449,0x044A,0x044B,0x044C,0x044D,0x044E,0x044F,
	}
	runes := make([]rune, 0, len(b))
	for _, c := range b {
		if c < 0x80 {
			runes = append(runes, rune(c))
		} else {
			runes = append(runes, table[c-0x80])
		}
	}
	return []byte(string(runes))
}

// isValidUTF8 проверяет является ли срез валидным UTF-8
func isValidUTF8(b []byte) bool {
	// Проверяем что json парсится без ошибок с UTF-8 точкой зрения
	for i := 0; i < len(b); i++ {
		if b[i] < 0x80 {
			continue
		}
		// Проверяем multi-byte sequence
		if b[i]&0xE0 == 0xC0 {
			if i+1 >= len(b) || b[i+1]&0xC0 != 0x80 {
				return false
			}
			i++
		} else if b[i]&0xF0 == 0xE0 {
			if i+2 >= len(b) || b[i+1]&0xC0 != 0x80 || b[i+2]&0xC0 != 0x80 {
				return false
			}
			i += 2
		} else {
			return false
		}
	}
	return true
}

var mirrorsMutex sync.RWMutex
var mirrors []*server.Server

var playersMutex sync.RWMutex
var players []*player.Player

type abuseMirror struct {
	server *server.Server
	item   Servers
	port   uint32
}

var abuseMirrorsMutex sync.Mutex
var abuseMirrors []abuseMirror

// глобальный счётчик использованных аккаунтов
var accountsIndex int
var accountsMutex sync.Mutex

var accounts []string
var tokens []string

func main() {
	log.Println("Product ID: mirrors-x-cs2go")
	log.Println("Product Version: 0.1.1-fixed")

	var err error

	os.Mkdir("cache", 0777)

	steam.InitServers(err)

	configPath := flag.String("config", "config.json", "Путь до файла с конфигом")
	accountsPath := flag.String("accounts", "accounts.txt", "Путь до файла с аккаунтами")
	tokensPath := flag.String("tokens", "tokens.txt", "Путь до файла с токенами")

	flag.Parse()

	var config = new(Config)

	content, err := ioutil.ReadFile(*configPath)
	if err != nil {
		log.Fatalf("Файл %s не найден\n", *configPath)
	}
	// Автоматически конвертируем Windows-1251 в UTF-8 если нужно
	if !isValidUTF8(content) {
		content = win1251toUTF8(content)
	}

	err = json.Unmarshal(content, &config)
	if err != nil {
		log.Fatalf("Неправильная структура конфига. %s\n", err)
	}

	accounts = utils.ReadFile(*accountsPath)
	tokens = utils.ReadFile(*tokensPath)

	log.Printf("Загружено токенов: %d, аккаунтов: %d\n", len(tokens), len(accounts))

	go heartbeatLoop()
	go loadServers(config)

	background()
}

// runServer запускает одно зеркало в своей goroutine
func runServer(item Servers, token string, port uint32, config *Config) {
	s := server.New()
	hostname := item.Hostname.Pick()
	s.SetHostname(hostname)
	s.SetMap(item.Map.Pick())
	s.SetMaxPlayers(item.MaxPlayers)
	s.SetPort(port)
	s.SetSecure(item.Secure)
	s.SetRegion(item.Region)
	s.SetBots(item.Bots)
	s.SetCSGOMod(config.CSGOMod)
	s.SetTags(item.Tags.Pick())
	s.SetAbuse(item.UseAbuse)
	s.SetPlayers(item.Players)
	s.SetVersion(item.Version)
	s.SetGameDir(item.GameDir)
	s.Connect()

	loggedOn := false

	// Читаем события до получения ответа на логин
	for event := range s.Events() {
		switch e := event.(type) {
		case *steam.ConnectedEvent:
			log.Printf("[server:%d] Подключён к Steam, отправка логина\n", port)
			s.Logon(token)
			continue // ждём следующего события

		case *server.LoggedOnEvent:
			if e.Result == 1 {
				log.Printf("[server:%d] Зеркало '%s' успешно залогинилось\n", port, hostname)
				mirrorsMutex.Lock()
				mirrors = append(mirrors, s)
				mirrorsMutex.Unlock()
				if item.UseAbuse {
					abuseMirrorsMutex.Lock()
					abuseMirrors = append(abuseMirrors, abuseMirror{server: s, item: item, port: port})
					abuseMirrorsMutex.Unlock()
				}
				s.StartA2S(item.ServerAddress)
				loggedOn = true
			} else {
				log.Printf("[server:%d] Ошибка логина зеркала '%s': result=%d\n", port, hostname, e.Result)
			}
			// В любом случае выходим из цикла ожидания
			goto doneLogin

		case steam.FatalErrorEvent:
			log.Printf("[server:%d] Фатальная ошибка: %v\n", port, e)
			goto doneLogin

		case error:
			log.Printf("[server:%d] Ошибка: %v\n", port, e)
			goto doneLogin
		}
	}

doneLogin:
	if !loggedOn {
		return
	}

	if !item.UseAbuse {
		// Обычные зеркала продолжают использовать отдельные аккаунты.
		runPlayersForServer(s, item)

		log.Printf("[server:%d] Отправка тикетов игроков\n", port)
		s.SendTickets()
	}
}

// nextAccount безопасно возвращает следующий аккаунт из пула
func nextAccount() (string, bool) {
	accountsMutex.Lock()
	defer accountsMutex.Unlock()
	if accountsIndex >= len(accounts) {
		return "", false
	}
	acc := accounts[accountsIndex]
	accountsIndex++
	return acc, true
}

// runPlayersForServer подключает нужное количество игроков к зеркалу
func runPlayersForServer(s *server.Server, item Servers) {
	var added uint8

	for added < item.Players {
		credLine, ok := nextAccount()
		if !ok {
			log.Printf("[player] Аккаунты закончились\n")
			break
		}

		credentials := strings.SplitN(credLine, ":", 2)
		if len(credentials) < 2 {
			log.Printf("[player] Неверный формат строки аккаунта: %s\n", credLine)
			continue
		}

		login := credentials[0]
		password := credentials[1]

		p := player.New()

		success := false
	playerLoop:
		for event := range p.Events() {
			switch e := event.(type) {
			case *steam.ConnectedEvent:
				p.Logon(login, password)
				continue

			case *player.LoggedOnEvent:
				if e.Result == 1 {
					log.Printf("[player] Аккаунт %s авторизован\n", login)
					playersMutex.Lock()
					players = append(players, p)
					playersMutex.Unlock()
					p.GetAppOwnershipTicket(730)
					continue
				}
				log.Printf("[player] Аккаунт %s: ошибка логина result=%d\n", login, e.Result)
				break playerLoop

			case *player.AppOwnershipTicketResponse:
				if e.Ticket != nil {
					result, err := p.AuthSessionTicket(e.Ticket)
					if err == nil {
						s.AddFakeClient(result.SteamId, result.Ticket, result.Crc)
						log.Printf("[player] Игрок %s добавлен в зеркало (порт %d)\n", login, item.Count)
						success = true
					} else {
						log.Printf("[player] Ошибка тикета %s: %v\n", login, err)
					}
				} else {
					log.Printf("[player] Пустой тикет у %s\n", login)
				}
				break playerLoop

			case steam.FatalErrorEvent:
				log.Printf("[player] Фатальная ошибка %s: %v\n", login, e)
				break playerLoop

			case error:
				log.Printf("[player] Ошибка %s: %v\n", login, e)
				break playerLoop
			}
		}

		if success {
			added++
		}

		// Если включён abuse-режим или есть боты — достаточно одного аккаунта
		if item.Bots > 0 || item.UseAbuse {
			break
		}
	}
}

func sendEmptyAbuseTickets(mirrors []abuseMirror) {
	for _, mirror := range mirrors {
		mirror.server.SendTickets()
	}
}

// runSharedAbuseAccount logs one account in once, then creates a separate
// authentication ticket for each use_abuse mirror.
func runSharedAbuseAccount(account string, mirrors []abuseMirror) {
	needsAccount := false
	for _, mirror := range mirrors {
		if mirror.item.Players > 0 {
			needsAccount = true
			break
		}
	}
	if !needsAccount {
		sendEmptyAbuseTickets(mirrors)
		return
	}

	if account == "" {
		log.Printf("[player] Для use_abuse нужен аккаунт, но accounts.txt пуст или аккаунт не выделен\n")
		sendEmptyAbuseTickets(mirrors)
		return
	}

	credentials := strings.SplitN(account, ":", 2)
	if len(credentials) < 2 {
		log.Printf("[player] Неверный формат строки аккаунта для use_abuse: %s\n", account)
		sendEmptyAbuseTickets(mirrors)
		return
	}

	login := credentials[0]
	p := player.New()

	for event := range p.Events() {
		switch e := event.(type) {
		case *steam.ConnectedEvent:
			p.Logon(login, credentials[1])
			continue

		case *player.LoggedOnEvent:
			if e.Result != 1 {
				log.Printf("[player] Аккаунт %s для use_abuse: ошибка логина result=%d\n", login, e.Result)
				sendEmptyAbuseTickets(mirrors)
				return
			}

			log.Printf("[player] Общий аккаунт %s авторизован для %d зеркал use_abuse\n", login, len(mirrors))
			playersMutex.Lock()
			players = append(players, p)
			playersMutex.Unlock()
			p.GetAppOwnershipTicket(730)

		case *player.AppOwnershipTicketResponse:
			if e.Ticket == nil {
				log.Printf("[player] Не удалось получить тикет приложения для аккаунта %s\n", login)
				sendEmptyAbuseTickets(mirrors)
				return
			}

			for _, mirror := range mirrors {
				if mirror.item.Players == 0 {
					mirror.server.SendTickets()
					continue
				}

				if !p.WaitForGameConnectToken(30 * time.Second) {
					log.Printf("[player] Не дождался токена подключения аккаунта %s для зеркала с портом %d\n", login, mirror.port)
					mirror.server.SendTickets()
					continue
				}

				result, err := p.AuthSessionTicket(e.Ticket)
				if err != nil {
					log.Printf("[player] Ошибка тикета %s: %v\n", login, err)
					mirror.server.SendTickets()
					continue
				}

				mirror.server.AddFakeClient(result.SteamId, result.Ticket, result.Crc)
				mirror.server.SendTickets()
				log.Printf("[player] Общий аккаунт %s добавлен в зеркало (порт %d)\n", login, mirror.port)
			}
			return

		case steam.FatalErrorEvent:
			log.Printf("[player] Фатальная ошибка общего аккаунта %s: %v\n", login, e)
			sendEmptyAbuseTickets(mirrors)
			return

		case error:
			log.Printf("[player] Ошибка общего аккаунта %s: %v\n", login, e)
			sendEmptyAbuseTickets(mirrors)
			return
		}
	}
}

func loadServers(config *Config) {
	sharedAbuseAccount := ""
	needsSharedAbuseAccount := false
	for _, item := range config.Servers {
		if item.UseAbuse && item.Players > 0 {
			needsSharedAbuseAccount = true
			break
		}
	}
	if needsSharedAbuseAccount {
		accountsMutex.Lock()
		if len(accounts) > 0 {
			sharedAbuseAccount = accounts[0]
			accountsIndex = 1
		}
		accountsMutex.Unlock()
	}

	var startPort = config.StartPort
	var portMutex sync.Mutex
	tokensCount := 0
	var tokensMutex sync.Mutex

	// Ограничиваем параллельные подключения к Steam (иначе бан)
	sem := make(chan struct{}, 5)

	var wg sync.WaitGroup

	for _, item := range config.Servers {
		for i := uint32(0); i < item.Count; i++ {
			tokensMutex.Lock()
			if tokensCount >= len(tokens) {
				tokensMutex.Unlock()
				log.Printf("[main] Токены закончились на %d зеркале\n", tokensCount)
				goto done
			}
			token := tokens[tokensCount]
			tokensCount++
			tokensMutex.Unlock()

			portMutex.Lock()
			port := startPort
			startPort++
			portMutex.Unlock()

			currentItem := item
			currentToken := token
			currentPort := port

			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				runServer(currentItem, currentToken, currentPort, config)
			}()

			// Небольшая пауза между запусками чтобы не получить rate-limit от Steam
			time.Sleep(100 * time.Millisecond)
		}
	}

done:
	wg.Wait()

	abuseMirrorsMutex.Lock()
	startedAbuseMirrors := append([]abuseMirror(nil), abuseMirrors...)
	abuseMirrorsMutex.Unlock()
	if len(startedAbuseMirrors) > 0 {
		runSharedAbuseAccount(sharedAbuseAccount, startedAbuseMirrors)
	}

	log.Printf("[main] Загрузка завершена. Зеркал: %d, игроков: %d\n", len(mirrors), len(players))
}

func heartbeatLoop() {
	heartbeat := time.NewTicker(10 * time.Second)
	for {
		<-heartbeat.C

		go func() {
			mirrorsMutex.RLock()
			count := len(mirrors)
			for _, m := range mirrors {
				go m.HeartBeat()
			}
			mirrorsMutex.RUnlock()
			if count > 0 {
				log.Printf("[heartbeat] Отправлено %d heartbeat зеркалам\n", count)
			}
		}()

		go func() {
			playersMutex.RLock()
			for _, p := range players {
				go p.HeartBeat()
			}
			playersMutex.RUnlock()
		}()
	}
}

func background() {
	fmt.Println("Нажми 'q' + Enter для выхода, или любой текст + Enter для статуса")

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		text := scanner.Text()
		if text == "q" {
			break
		}
		mirrorsMutex.RLock()
		playersMutex.RLock()
		fmt.Printf("=== Статус: зеркал=%d, игроков=%d ===\n", len(mirrors), len(players))
		playersMutex.RUnlock()
		mirrorsMutex.RUnlock()
	}
}
