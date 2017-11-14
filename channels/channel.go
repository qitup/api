package channels

import "github.com/olahol/melody"

type Channel interface {
	HandleConnect(s *melody.Session)
	HandleDisconnect(s *melody.Session)
	HandleMessage(s *melody.Session, data []byte)
}

type multiChannel struct {
	Channel
	channels map[string]Channel
}

var Mutli = multiChannel{channels: make(map[string]Channel)}

func(m *multiChannel) Register(identifier string, channel Channel) {
	m.channels[identifier] = channel
}

func(m *multiChannel) HandleConnect(s *melody.Session) {
	if channel_id, ok := s.Get("channel"); ok {
		if channel, ok := m.channels[channel_id.(string)]; ok {
			channel.HandleConnect(s)
		}
	} else {
		panic("Invalid channel id" + channel_id.(string))
	}
}

func(m *multiChannel) HandleDisconnect(s *melody.Session) {
	if channel_id, ok := s.Get("channel"); ok {
		if channel, ok := m.channels[channel_id.(string)]; ok {
			channel.HandleDisconnect(s)
		}
	} else {
		panic("Invalid channel id" + channel_id.(string))
	}
}

func(m *multiChannel) HandleMessage(s *melody.Session, data []byte) {
	if channel_id, ok := s.Get("channel"); ok {
		if channel, ok := m.channels[channel_id.(string)]; ok {
			channel.HandleMessage(s, data)
		}
	} else {
		panic("Invalid channel id" + channel_id.(string))
	}
}