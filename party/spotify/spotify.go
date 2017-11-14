package spotify

import (
	"github.com/zmb3/spotify"
	"dubclan/api/party"
	"golang.org/x/oauth2"
	//"github.com/VividCortex/multitick"
	"time"
	"log"
)

var (
	HostScopes = []string{"user-library-read", "user-read-private", "user-read-playback-state", "user-modify-playback-state", "user-read-currently-playing"}

	authenticator = spotify.Authenticator{}
)

type SpotifyPlayer struct {
	party.Player
	client         spotify.Client
	playback_state *spotify.PlayerState
	device_id      *string

	// Send true when a party becomes inactive
	inactive chan bool
}

func New(token *oauth2.Token, device_id *string, event_pipe chan string) SpotifyPlayer {
	return SpotifyPlayer{
		client:         authenticator.NewClient(token),
		playback_state: nil,
		device_id:      device_id,
		inactive:       make(chan bool),
	}
}

func (p *SpotifyPlayer) Play(item []party.Item) (error) {
	//opt := spotify.PlayOptions{
	//
	//}
	//
	//p.client.PlayOpt(spotify.PlayOptions{})
	return nil
}

func (p *SpotifyPlayer) Pause() (error) {
	//p.client.PauseOpt()
	return nil
}

func (p *SpotifyPlayer) Next() (error) {
	panic("implement me")
}

func (p *SpotifyPlayer) Previous() (error) {
	panic("implement me")
}

// Update the state of the players until a session becomes inactive
func (p *SpotifyPlayer) poll(ticked <-chan time.Time) {
	for {
		select {
		// Update states of our players
		case tick := <-ticked:
			log.Println("TICKED", tick)
			//for _, player := range s.Players {
			//	player.UpdateState()
			//}
			break

		//case done := <-p.Inactive:
		//	if done {
		//		return
		//	}
		//	break
		}
	}
}

func (p *SpotifyPlayer) UpdateState() (error) {
	panic("implement me")
}
