package a2s

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"
)

type ServerInfo struct {
	SteamID       *uint64
	Port          uint16
	Hostname      string
	Map           string
	Folder        string
	Game          string
	Players       uint8
	MaxPlayers    uint8
	Bots          uint8
	Secure        bool
	Tags          string
	ServerAddress string
}

type session struct {
	clientAddr *net.UDPAddr
	lastSeen   time.Time
}

func ListenAndServe(port uint32, info *ServerInfo) {
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	pc, err := net.ListenPacket("udp4", addr)
	if err != nil {
		log.Printf("[a2s:%d] BIND FAILED: %v\n", port, err)
		return
	}
	defer pc.Close()

	// Генерируем случайный challenge для этого сервера
	challenge := make([]byte, 4)
	binary.LittleEndian.PutUint32(challenge, rand.Uint32())

	var realConn *net.UDPConn
	if info.ServerAddress != "" {
		realAddr, err := net.ResolveUDPAddr("udp4", info.ServerAddress)
		if err == nil {
			realConn, err = net.DialUDP("udp4", nil, realAddr)
			if err != nil {
				realConn = nil
			}
		}
	}

	if realConn != nil {
		log.Printf("[a2s:%d] UDP listening (proxy -> %s)\n", port, info.ServerAddress)
		defer realConn.Close()
	} else {
		log.Printf("[a2s:%d] UDP listening\n", port)
	}

	sessions := make(map[string]*session)
	var sessionsMu sync.Mutex

	// Читаем ответы от реального сервера
	if realConn != nil {
		go func() {
			buf := make([]byte, 8192)
			for {
				n, err := realConn.Read(buf)
				if err != nil {
					return
				}
				data := make([]byte, n)
				copy(data, buf[:n])

				// Подменяем A2S_INFO ответ
				if n > 5 && data[0] == 0xFF && data[1] == 0xFF && data[2] == 0xFF && data[3] == 0xFF && data[4] == 0x49 {
					data = buildInfoResponse(info)
				}

				sessionsMu.Lock()
				for _, s := range sessions {
					if time.Since(s.lastSeen) < 30*time.Second {
						pc.WriteTo(data, s.clientAddr)
					}
				}
				sessionsMu.Unlock()
			}
		}()
	}

	buf := make([]byte, 8192)
	for {
		n, remote, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		data := buf[:n]

		clientKey := remote.String()
		sessionsMu.Lock()
		if s, ok := sessions[clientKey]; ok {
			s.lastSeen = time.Now()
		} else {
			sessions[clientKey] = &session{clientAddr: remote.(*net.UDPAddr), lastSeen: time.Now()}
		}
		sessionsMu.Unlock()


		if n < 5 || data[0] != 0xFF || data[1] != 0xFF || data[2] != 0xFF || data[3] != 0xFF {
			if realConn != nil {
				realConn.Write(data)
			}
			continue
		}

		switch data[4] {
		case 0x54: // A2S_INFO request
			if n >= 29 {
				// Запрос с challenge (новый протокол CS2) — отвечаем INFO
				clientChallenge := data[n-4:]
				pc.WriteTo(buildInfoResponseWithChallenge(info, clientChallenge), remote)
			} else {
				// Запрос без challenge — отвечаем challenge
				resp := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x41,
					challenge[0], challenge[1], challenge[2], challenge[3]}
				pc.WriteTo(resp, remote)
			}

		case 0x55: // A2S_PLAYER
			if n >= 9 {
				pc.WriteTo(buildPlayerResponse(), remote)
			} else {
				pc.WriteTo([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0x41,
					challenge[0], challenge[1], challenge[2], challenge[3]}, remote)
			}

		case 0x56: // A2S_RULES
			if n >= 9 {
				pc.WriteTo(buildRulesResponse(), remote)
			} else {
				pc.WriteTo([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0x41,
					challenge[0], challenge[1], challenge[2], challenge[3]}, remote)
			}

		case 0x57: // challenge request
			pc.WriteTo([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0x41,
				challenge[0], challenge[1], challenge[2], challenge[3]}, remote)

		default:
			// Игровой трафик — проксируем
			if realConn != nil {
				realConn.Write(data)
			}
		}
	}
}

func buildInfoResponse(info *ServerInfo) []byte {
	return buildInfoResponseWithChallenge(info, nil)
}

func buildInfoResponseWithChallenge(info *ServerInfo, challenge []byte) []byte {
	buf := new(bytes.Buffer)
	buf.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0x49})
	buf.WriteByte(17) // protocol version
	writeString(buf, info.Hostname)
	writeString(buf, info.Map)
	writeString(buf, info.Folder)
	writeString(buf, info.Game)
	buf.Write([]byte{0xDA, 0x02}) // AppID 730 little-endian
	buf.WriteByte(info.Players)
	buf.WriteByte(info.MaxPlayers)
	buf.WriteByte(info.Bots)
	buf.WriteByte('d') // dedicated
	buf.WriteByte('l') // linux
	buf.WriteByte(0)   // public
	if info.Secure {
		buf.WriteByte(1)
	} else {
		buf.WriteByte(0)
	}
	writeString(buf, "13992")
	// EDF flags: 0x80=port, 0x10=SteamID, 0x20=tags
	edf := byte(0x80 | 0x10 | 0x20)
	buf.WriteByte(edf)
	// 0x80: game port (uint16 LE)
	buf.WriteByte(byte(info.Port & 0xFF))
	buf.WriteByte(byte(info.Port >> 8))
	// 0x10: SteamID (uint64 LE)
	var sid uint64
	if info.SteamID != nil {
		sid = *info.SteamID
	}
	for i := 0; i < 8; i++ {
		buf.WriteByte(byte(sid & 0xFF))
		sid >>= 8
	}
	// 0x20: tags
	writeString(buf, info.Tags)
	return buf.Bytes()
}

func buildPlayerResponse() []byte {
	return []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x44, 0x00}
}

func buildRulesResponse() []byte {
	return []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x45, 0x00, 0x00}
}

func writeString(buf *bytes.Buffer, s string) {
	buf.WriteString(s)
	buf.WriteByte(0x00)
}
