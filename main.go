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

var mirrorsMutex sync.RWMutex
var mirrors []*server.Server

var playersMutex sync.RWMutex
var players []*player.Player

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

	// Запускаем фейковых игроков
	runPlayersForServer(s, item)

	// Отправляем все тикеты серверу Steam
	log.Printf("[server:%d] Отправка тикетов игроков\n", port)
	s.SendTickets()
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

func loadServers(config *Config) {
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
