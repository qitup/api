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
	current_items  []models.Item
	// Close this channel when a party becomes inactive
	stop   chan bool
	ticker *time.Ticker
}

func New(emitter *emitter.Emitter, token *oauth2.Token, device_id *string) (*SpotifyPlayer, error) {

	return &SpotifyPlayer{
		emitter:        emitter,
		client:         authenticator.NewClient(token),
		playback_state: nil,
		device_id:      device_id,
	}, nil
}

func (p *SpotifyPlayer) Stop() {
	p.stopPolling()
}

func (p *SpotifyPlayer) Play(items []models.Item) (error) {
	if items == nil && len(p.current_items) > 0 {
		items = p.current_items
	}

	uris := []spotify.URI{}
	for _, item := range items {
		switch item.(type) {
		case *models.SpotifyTrack:
			track := item.(*models.SpotifyTrack)
			uris = append(uris, track.URI)
			break
		}
	}

	opt := spotify.PlayOptions{
		URIs: uris,
		PlaybackOffset: &spotify.PlaybackOffset{
			URI: uris[0],
		},
	}

	if err := p.client.PlayOpt(&opt); err != nil {
		return err
	}
	p.emitter.Emit("player.play", true)

	p.current_items = items
	p.startPolling(POLL_INTERVAL)
	return nil
}

func (p *SpotifyPlayer) Resume() (error) {
	// SPOTIFY RETURNS A SERVER ERROR IF THERE IS A TRACK CURRENTLY PLAYING
	if err := p.client.Play(); err != nil {
		return err
	}
	p.emitter.Emit("player.play", false)

	p.startPolling(POLL_INTERVAL)
	return nil
}

func (p *SpotifyPlayer) Pause() (error) {
	if err := p.client.Pause(); err != nil {
		return err
	}
	p.emitter.Emit("player.pause")

	p.stopPolling()
	return nil
}

func (p *SpotifyPlayer) Next() (error) {
	//if err := p.client.Next(); err != nil {
	//	return err
	//}
	//
	//p.startPolling(POLL_INTERVAL)
	//return nil
	panic("NOT IMPLEMENTED")
}

func (p *SpotifyPlayer) Previous() (error) {
	panic("implement me")
}

func (p *SpotifyPlayer) HasItems() (bool) {
	return len(p.current_items) > 0
}

func (p *SpotifyPlayer) stopPolling() {
	if p.ticker != nil {
		// Stop delivering ticks
		p.ticker.Stop()
		close(p.stop)
		p.ticker = nil
	}
}

func (p *SpotifyPlayer) startPolling(interval time.Duration) {
	if p.ticker == nil {
		p.stop = make(chan bool)
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

		case <-p.stop:
			return
		}
	}
}

func (p *SpotifyPlayer) current() spotify.URI {
	if len(p.current_items) < 1 {
		return ""
	}

	item := p.current_items[0]
	switch item.(type) {
	case *models.SpotifyTrack:
		track := item.(*models.SpotifyTrack)
		return track.URI
		break
	}

	return ""
}

func (p *SpotifyPlayer) peekNext() spotify.URI {
	if len(p.current_items) <= 1 {
		return ""
	}

	item := p.current_items[1]
	switch item.(type) {
	case *models.SpotifyTrack:
		track := item.(*models.SpotifyTrack)
		return track.URI
		break
	}

	return ""
}

func (p *SpotifyPlayer) UpdateState(new_state *spotify.PlayerState) (error) {
	if p.playback_state == nil {
		if new_state.Playing {
			if new_state.Item.URI == p.current() {
				// Playback started
				p.emitter.Emit("player.play", false)
			} else {
				p.emitter.Emit("player.interrupted")
			}
		}
	} else if p.playback_state.Playing && !new_state.Playing {
		if p.playback_state.Item.ID == new_state.Item.ID {
			if new_state.Progress == 0 {
				var prev models.Item
				prev, p.current_items = p.current_items[0], p.current_items[1:]
				prev.Done()
				p.emitter.Emit("player.track_finished", true)
			} else {
				p.emitter.Emit("player.paused")
			}
		} else {
			if new_state.Progress == 0 {
				var prev models.Item
				prev, p.current_items = p.current_items[0], p.current_items[1:]
				prev.Done()
				p.emitter.Emit("player.track_finished", true)
			} else {
				p.emitter.Emit("player.interrupted")
			}
		}
	} else if p.playback_state.Item.ID != new_state.Item.ID {
		if !p.HasItems() {
			// playback of something else has been started somewhere
			p.emitter.Emit("player.interrupted")
		} else if p.current() == new_state.Item.URI {
			p.emitter.Emit("player.play", false)
		} else if p.peekNext() == new_state.Item.URI {
			var prev models.Item
			// Track changed to next item
			prev, p.current_items = p.current_items[0], p.current_items[1:]
			prev.Done()
			p.emitter.Emit("player.track_finished", false)
		} else if len(p.current_items) == 1 && p.playback_state.Playing && !new_state.Playing {
			var prev models.Item
			prev, p.current_items = p.current_items[0], p.current_items[1:]
			prev.Done()
			p.emitter.Emit("player.track_finished", true)
		} else {
			// playback has been started somewhere else
			p.emitter.Emit("player.interrupted")
		}
	} else {
		// Update currently playing track
		if !p.playback_state.Playing && new_state.Playing {
			// Playback started
			p.emitter.Emit("player.play", false)
		} else if p.playback_state.Playing && !new_state.Playing {
			p.emitter.Emit("player.pause")
		}
	}

	p.playback_state = new_state

	return nil
}
