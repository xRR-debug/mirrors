package steam

import (
	"encoding/json"
	"fmt"
	"github.com/edwingeng/deque/v2"
	"log"
	"net/http"
	"sync"
)

var servers *deque.Deque[string] = nil
var serversRWMutex sync.RWMutex

func InitServers(err error) {
	servers = deque.NewDeque[string]()

	if err == nil {
		getServers()
	}
}

func GetServer() string {
	if servers == nil || servers.IsEmpty() {
		log.Fatalf("Empty Servers")
	}

	serversRWMutex.RLock()
	server, _ := servers.Front()
	serversRWMutex.RUnlock()

	return server
}

func BlockServerAndBackNewServer(server string) string {
	serversRWMutex.Lock()
	fmt.Println("Block IP")
	index := -1

	servers.Range(func(i int, v string) bool {
		if v == server {
			index = i
			return true
		}

		return false
	})

	if index != -1 {
		servers.Remove(index)
	}

	serversRWMutex.Unlock()

	return GetServer()
}

func getServers() {
	for i := 0; i < 15; i++ {
		client := new(http.Client)
		resp, err := client.Get(fmt.Sprintf("https://api.steampowered.com/ISteamDirectory/GetCMListForConnect/v1/?cellId=%d", i))

		if err != nil {
			log.Fatalf("[client] Error getting servers")
		}

		data := Response{}

		if err = json.NewDecoder(resp.Body).Decode(&data); err != nil {
			log.Fatalf("[client] Error parse servers")
		}

		if len(data.Response.ServerList) == 0 {
			log.Fatalf("[client] Steam returned zero servers")
		}

		for _, server := range data.Response.ServerList {
			serversRWMutex.Lock()
			if server.Type == "websockets" {
				servers.PushBack(server.EndPoint)
			}
			serversRWMutex.Unlock()
		}
	}
}

type Response struct {
	Response struct {
		ServerList []ServerList `json:"serverlist"`
	} `json:"response"`
}

type ServerList struct {
	EndPoint string `json:"endpoint"`
	Type     string `json:"type"`
}
