package server

import (
	"wirstaff.com/mirrors/protocol"
	"wirstaff.com/mirrors/protocol/protobuf"
	"wirstaff.com/mirrors/protocol/steamlang"
)

type LoggedOnEvent struct {
	Result steamlang.EResult
}

func (s *Server) HandlePacket(packet *protocol.Packet) {
	switch packet.EMsg {
	case steamlang.EMsg_ClientLogOnResponse:
		s.handleLogOnResponse(packet)
	}
}
func (s *Server) handleLogOnResponse(packet *protocol.Packet) {
	body := new(protobuf.CMsgClientLogonResponse)
	msg := packet.ReadProtoMsg(body)

	if body.GetEresult() == 1 {
		s.client.SetSessionId(msg.Header.Proto.GetClientSessionid())
		s.client.SetSteamId(msg.Header.Proto.GetSteamid())
		s.steamID = msg.Header.Proto.GetSteamid()

		s.sendServerType()
		s.sendServerData()
	}

	s.client.Emit(&LoggedOnEvent{Result: steamlang.EResult(body.GetEresult())})
}
