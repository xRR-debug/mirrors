package steam

import (
	"wirstaff.com/mirrors/protocol"
)

func (c *Client) readLoop() {
	for {
		c.mutex.RLock()
		conn := c.conn
		c.mutex.RUnlock()
		if conn == nil {
			return
		}

		_, packet, err := c.conn.ReadMessage()

		if err != nil {
			c.Fatalf("Error reading from the connection: %v", err)
			return
		}

		buffer, err := protocol.NewPacket(packet)

		if err != nil {
			continue
		}

		c.handlePacket(buffer)
	}
}
