package player

import (
	"github.com/zmb3/spotify"
	"dubclan/api/party"
)

type SpotifyPlayer struct {
	Client spotify.Client
	State spotify.PlayerState
}

func (p *SpotifyPlayer) Play(item party.Item) error {
	//switch item.(type) {
	//case party.SpotifyTrack:
	//	i := party.SpotifyTrack(item)
	//
	//	log.Println(i.URI)
	//	break
	//}

	return nil
}



