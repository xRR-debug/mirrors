package steam

import (
	"bytes"
	"github.com/gorilla/websocket"
	"net/url"
	"sync"
	"sync/atomic"
	"wirstaff.com/mirrors/protocol"
	"wirstaff.com/mirrors/steamid"
)

type Client struct {
	events        chan interface{}
	handlers      []PacketHandler
	handlersMutex sync.RWMutex

	conn  *websocket.Conn
	mutex sync.RWMutex

	sessionId int32
	steamId   uint64
}

func New() *Client {
	client := &Client{
		events: make(chan interface{}, 3),
	}

	return client
}

func (c *Client) Connect() {
	c.Disconnect()

	server := GetServer()

	for {
		u := url.URL{Scheme: "wss", Host: server, Path: "/cmsocket/"}

		var dialer = websocket.Dialer{}

		var err error
		c.conn, _, err = dialer.Dial(u.String(), nil)

		if err != nil {
			server = BlockServerAndBackNewServer(server)
			continue
		}

		c.Emit(&ConnectedEvent{})
		break
	}

	go c.readLoop()
}

func (c *Client) Disconnect() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.conn == nil {
		return
	}

	c.conn.Close()
	c.conn = nil
	c.Emit(&DisconnectedEvent{})
}

func (c *Client) Write(msg protocol.IMsg) {
	if cm, ok := msg.(protocol.IClientMsg); ok {
		cm.SetSessionId(c.GetSessionId())
		cm.SetSteamId(c.GetSteamId())
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.conn == nil {
		return
	}

	buffer := new(bytes.Buffer)

	err := msg.Serialize(buffer)
	if err != nil {
		buffer.Reset()
		c.Fatalf("Error serializing message %v: %v", msg, err)
		return
	}

	err = c.conn.WriteMessage(websocket.BinaryMessage, buffer.Bytes())

	buffer.Reset()

	if err != nil {
		c.Fatalf("Error writing message %v: %v", msg, err)
		return
	}
}

func (c *Client) GetSessionId() int32 {
	return c.sessionId
}

func (c *Client) GetSteamId() steamid.SteamId {
	return steamid.SteamId(atomic.LoadUint64(&c.steamId))
}

func (c *Client) SetSessionId(sessionId int32) {
	c.sessionId = sessionId
}

func (c *Client) SetSteamId(steamId uint64) {
	c.steamId = steamId
}
