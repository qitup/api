package spotify

import (
	"github.com/zmb3/spotify"
	"time"
	"log"
	"dubclan/api/models"
	"sync"
	"github.com/urfave/cli"
	spotify_provider "github.com/markbates/goth/providers/spotify"
	"github.com/markbates/goth"
	"github.com/olebedev/emitter"
	"golang.org/x/oauth2"
)

var (
	hostScopes    = []string{"user-library-read", "user-read-private", "user-read-playback-state", "user-modify-playback-state", "user-read-currently-playing"}
	authenticator = spotify.NewAuthenticator("", hostScopes...)

	provider goth.Provider
)

const (
	POLL_INTERVAL = 5 * time.Second
)

func InitProvider(callback_url string, cli *cli.Context) goth.Provider {
	provider = spotify_provider.New(
		cli.String("spotify-id"),
		cli.String("spotify-secret"),
		callback_url+"/auth/spotify/callback",
		hostScopes...,
	)

	return provider
}

type SpotifyPlayer struct {
	emitter        *emitter.Emitter
	client         spotify.Client
	refreshing     sync.RWMutex // Locked whilst refreshing host's access token
	playback_state *spotify.PlayerState
	device_id      *string
	current_tracks []spotify.URI
	cursor         int

	// Send true to this channel when a party becomes inactive
	stop   chan bool
	ticker *time.Ticker
}

func New(emitter *emitter.Emitter, token *oauth2.Token, device_id *string) (*SpotifyPlayer, error) {

	return &SpotifyPlayer{
		emitter:        emitter,
		client:         authenticator.NewClient(token),
		playback_state: nil,
		device_id:      device_id,
		stop:           make(chan bool),
		cursor:         -1,
	}, nil
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

	opt := spotify.PlayOptions{
		URIs: uris,
		PlaybackOffset: &spotify.PlaybackOffset{
			URI: uris[0],
		},
	}

	if err := p.client.PlayOpt(&opt); err != nil {
		return err
	}
	p.current_tracks = uris
	p.cursor = 0
	p.startPolling(POLL_INTERVAL)
	return nil
}

func (p *SpotifyPlayer) Resume() (error) {
	// SPOTIFY RETURNS A SERVER ERROR IF THERE IS A TRACK CURRENTLY PLAYING
	if err := p.client.Play(); err != nil {
		return err
	}

	p.startPolling(POLL_INTERVAL)
	return nil
}

func (p *SpotifyPlayer) Pause() (error) {
	if err := p.client.Pause(); err != nil {
		return err
	}

	p.stopPolling()
	return nil
}

func (p *SpotifyPlayer) Next() (error) {
	if err := p.client.Next(); err != nil {
		return err
	}

	p.startPolling(POLL_INTERVAL)
	return nil
}

func (p *SpotifyPlayer) Previous() (error) {
	panic("implement me")
}

func (p *SpotifyPlayer) HasItems() (bool) {
	return p.cursor != -1 && p.cursor <= len(p.current_tracks)
}

func (p *SpotifyPlayer) stopPolling() {
	if p.ticker != nil {
		// Stop delivering ticks
		p.ticker.Stop()
		p.stop <- true
		p.ticker = nil
	}
}

func (p *SpotifyPlayer) startPolling(interval time.Duration) {
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
			state, err := p.client.PlayerState()

			if err != nil {
				log.Println("Failed getting spotify player state ", err)
				return
			}

			// TODO: Update player's state
			if state != nil {
				p.UpdateState(state)
			}

			break

		case done := <-p.stop:
			if done {
				return
			}
			break
		}
	}
}

func (p *SpotifyPlayer) UpdateState(new_state *spotify.PlayerState) (error) {
	log.Println("PREV:", p.playback_state)
	log.Println("NEW:", new_state)

	if p.playback_state == nil {
		if new_state.Playing {
			// Playback started
			p.emitter.Emit("player.play")
		}
		p.playback_state = new_state
	} else if p.playback_state.Item.ID != new_state.Item.ID {
		log.Println("PREV URI:", p.playback_state.Item.URI)
		log.Println("NEW URI:", new_state.Item.URI)
		log.Println(p.cursor, p.current_tracks)

		if !p.HasItems() || p.cursor == len(p.current_tracks) {
			// playback has been started somewhere else
			p.emitter.Emit("player.interrupted")
		} else if p.current_tracks[p.cursor+1] == new_state.Item.URI {
			// Track changed to next item
			p.cursor++
			if p.cursor == len(p.current_tracks) {
				p.emitter.Emit("player.empty")
			} else {
				p.emitter.Emit("player.changed")
			}
		} else {
			// playback has been started somewhere else
			p.emitter.Emit("player.interrupted")
		}
		p.playback_state = new_state
	} else {
		// Update currently playing track
		if !p.playback_state.Playing && new_state.Playing {
			// Playback started
			p.emitter.Emit("player.play")
		}

		p.playback_state = new_state
	}

	return nil
}
