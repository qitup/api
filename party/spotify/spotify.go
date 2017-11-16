package spotify

import (
	"github.com/zmb3/spotify"
	"golang.org/x/oauth2"
	"time"
	"log"
	"dubclan/api/models"
)

var (
	HostScopes = []string{"user-library-read", "user-read-private", "user-read-playback-state", "user-modify-playback-state", "user-read-currently-playing"}

	authenticator = spotify.Authenticator{}
)

type SpotifyPlayer struct {
	client         spotify.Client
	playback_state *spotify.PlayerState
	device_id      *string

	// Send true when a party becomes inactive
	stop   chan bool
	ticker *time.Ticker
}

func New(token *oauth2.Token, device_id *string) *SpotifyPlayer {
	return &SpotifyPlayer{
		client:         authenticator.NewClient(token),
		playback_state: nil,
		device_id:      device_id,
		stop:           make(chan bool),
	}
}

func (p *SpotifyPlayer) Play(items []models.Item) (error) {
	uris := []spotify.URI{}
	for _, item := range items {
		switch item.(type) {
		case *models.SpotifyTrack:
			track := item.(*models.SpotifyTrack)
			uris = append(uris, track.URI)
			break
		}
	}

	log.Println(uris)

	opt := spotify.PlayOptions{nil, nil, uris, nil}

	if err := p.client.PlayOpt(&opt); err != nil {
		return err
	}

	p.StartPolling(5 * time.Second)
	return nil
}

func (p *SpotifyPlayer) Pause() (error) {
	if err := p.client.Pause(); err != nil {
		return err
	}
	p.StopPolling()
	return nil
}

func (p *SpotifyPlayer) Next() (error) {
	panic("implement me")
}

func (p *SpotifyPlayer) Previous() (error) {
	panic("implement me")
}

func (p *SpotifyPlayer) StopPolling() {
	if p.ticker != nil {
		// Stop delivering ticks
		p.ticker.Stop()
		p.stop <- true
		p.ticker = nil
	}
}

func (p *SpotifyPlayer) StartPolling(interval time.Duration) {
	if p.ticker == nil {
		p.ticker = time.NewTicker(interval)
		go p.poll()
	}
}

// Update the state of the players until a session becomes inactive
func (p *SpotifyPlayer) poll() {
	for {
		select {
		// Update states of our players
		case <-p.ticker.C:
			if _, err := p.client.PlayerState(); err != nil {
				log.Println("Failed getting spotify player state ", err)
				return
			} else {
				//log.Println(state.CurrentlyPlaying)
			}

			//for _, player := range s.Players {
			//	player.UpdateState()
			//}
			break

		case done := <-p.stop:
			if done {
				return
			}
			break
		}
	}
}

func (p *SpotifyPlayer) UpdateState() (error) {
	panic("implement me")
}
