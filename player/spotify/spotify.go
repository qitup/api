package spotify

import (
	"log"
	"sync"
	"time"

	"dubclan/api/models"
	"dubclan/api/player"

	"github.com/zmb3/spotify"

	"github.com/markbates/goth"
	spotifyProvider "github.com/markbates/goth/providers/spotify"
	"github.com/olebedev/emitter"
	"github.com/urfave/cli"
	"golang.org/x/oauth2"
)

var (
	hostScopes    = []string{"user-library-read", "user-read-private", "user-read-playback-state", "user-modify-playback-state", "user-read-currently-playing"}
	authenticator = spotify.NewAuthenticator("", hostScopes...)

	provider goth.Provider
)

const (
	PollInterval = 5 * time.Second
)

func InitProvider(callbackUrl string, cli *cli.Context) goth.Provider {
	provider = spotifyProvider.New(
		cli.String("spotify-id"),
		cli.String("spotify-secret"),
		callbackUrl+"/auth/spotify/callback",
		hostScopes...,
	)

	return provider
}

type SpotifyPlayer struct {
	emitter       *emitter.Emitter
	client        spotify.Client
	refreshing    sync.RWMutex // Locked whilst refreshing host's access token
	playbackState *spotify.PlayerState
	deviceId      *string
	currentItems  []models.Item
	stop          chan bool // Close this channel when a party becomes inactive
	ticker        *time.Ticker
	state         int
}

func New(emitter *emitter.Emitter, token *oauth2.Token, deviceId *string) (*SpotifyPlayer, error) {

	return &SpotifyPlayer{
		emitter:       emitter,
		client:        authenticator.NewClient(token),
		playbackState: nil,
		deviceId:      deviceId,
		state:         player.READY,
	}, nil
}

func (p *SpotifyPlayer) Stop() {
	p.stopPolling()
}

func (p *SpotifyPlayer) Play(items []models.Item) (error) {
	switch p.state {
	case player.INTERRUPTED:
		items = p.currentItems
		break
	case player.PAUSED:
		return p.Resume()
	}

	var uris []spotify.URI
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
	p.currentItems = items
	p.state = player.PLAYING
	p.currentItems[0].Play()
	p.emitter.Emit("player.play")

	p.startPolling(PollInterval)
	return nil
}

func (p *SpotifyPlayer) Resume() (error) {
	// SPOTIFY RETURNS A SERVER ERROR IF THERE IS A TRACK CURRENTLY PLAYING
	if err := p.client.Play(); err != nil {
		return err
	}
	p.state = player.PLAYING
	p.currentItems[0].Play()
	p.emitter.Emit("player.play")

	p.startPolling(PollInterval)
	return nil
}

func (p *SpotifyPlayer) Pause() (error) {
	if err := p.client.Pause(); err != nil {
		return err
	}
	p.state = player.PAUSED
	p.currentItems[0].Pause()
	p.emitter.Emit("player.pause")

	p.stopPolling()
	return nil
}

func (p *SpotifyPlayer) Next() (error) {
	//if err := p.client.Next(); err != nil {
	//	return err
	//}
	//
	//p.startPolling(PollInterval)
	//return nil
	panic("NOT IMPLEMENTED")
}

func (p *SpotifyPlayer) Previous() (error) {
	panic("implement me")
}

func (p *SpotifyPlayer) HasItems() (bool) {
	return len(p.currentItems) > 0
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
		case _, ok := <-p.ticker.C:
			if !ok {
				return
			}

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
	if len(p.currentItems) < 1 {
		return ""
	}

	item := p.currentItems[0]
	switch item.(type) {
	case *models.SpotifyTrack:
		track := item.(*models.SpotifyTrack)
		return track.URI
		break
	}

	return ""
}

func (p *SpotifyPlayer) peekNext() spotify.URI {
	if len(p.currentItems) <= 1 {
		return ""
	}

	item := p.currentItems[1]
	switch item.(type) {
	case *models.SpotifyTrack:
		track := item.(*models.SpotifyTrack)
		return track.URI
		break
	}

	return ""
}

func (p *SpotifyPlayer) GetState() int {
	return p.state
}

func (p *SpotifyPlayer) UpdateState(newState *spotify.PlayerState) (error) {
	if p.playbackState == nil {
		if newState.Playing {
			if newState.Item.URI == p.current() {
				// Playback started
				p.state = player.PLAYING
				p.emitter.Emit("player.play", false)
			} else {
				p.state = player.INTERRUPTED
				p.emitter.Emit("player.interrupted")
			}
		}
	} else if p.playbackState.Playing && !newState.Playing {
		if p.playbackState.Item.ID == newState.Item.ID {
			if newState.Progress == 0 {
				var prev models.Item
				prev, p.currentItems = p.currentItems[0], p.currentItems[1:]
				prev.Done()
				p.emitter.Emit("player.track_finished", true)
			} else {
				p.state = player.PAUSED
				p.emitter.Emit("player.paused")
			}
		} else {
			if newState.Progress == 0 {
				var prev models.Item
				prev, p.currentItems = p.currentItems[0], p.currentItems[1:]
				prev.Done()
				p.emitter.Emit("player.track_finished", true)
			} else {
				p.state = player.INTERRUPTED
				p.emitter.Emit("player.interrupted")
			}
		}
	} else if p.playbackState.Item.ID != newState.Item.ID {
		if !p.HasItems() {
			// playback of something else has been started somewhere
			p.state = player.INTERRUPTED
			p.emitter.Emit("player.interrupted")
		} else if p.current() == newState.Item.URI {
			p.state = player.PLAYING
			p.emitter.Emit("player.play", false)
		} else if p.peekNext() == newState.Item.URI {
			var prev models.Item
			// Track changed to next item
			prev, p.currentItems = p.currentItems[0], p.currentItems[1:]
			prev.Done()
			p.emitter.Emit("player.track_finished", false)
		} else if len(p.currentItems) == 1 && p.playbackState.Playing && !newState.Playing {
			var prev models.Item
			prev, p.currentItems = p.currentItems[0], p.currentItems[1:]
			prev.Done()
			p.emitter.Emit("player.track_finished", true)
		} else {
			// playback has been started somewhere else
			p.state = player.INTERRUPTED
			p.emitter.Emit("player.interrupted")
		}
	} else {
		// Update currently playing track
		if !p.playbackState.Playing && newState.Playing {
			// Playback started
			p.state = player.PLAYING
			p.emitter.Emit("player.play", false)
		} else if p.playbackState.Playing && !newState.Playing {
			p.state = player.PAUSED
			p.emitter.Emit("player.pause")
		}
	}

	p.playbackState = newState

	return nil
}
