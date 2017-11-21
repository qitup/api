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
)

var (
	hostScopes    = []string{"user-library-read", "user-read-private", "user-read-playback-state", "user-modify-playback-state", "user-read-currently-playing"}
	authenticator = spotify.Authenticator{}

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
	identity       *models.RefreshableIdentity
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

func New(identity *models.RefreshableIdentity, device_id *string) (*SpotifyPlayer, error) {
	token, err := identity.GetToken(provider)

	if err != nil {
		return nil, err
	}

	return &SpotifyPlayer{
		identity:       identity,
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

	opt := spotify.PlayOptions{nil, nil, uris, nil}

	err := p.actionWithRefresh(func() error {
		return p.client.PlayOpt(&opt)
	})

	if err != nil {
		return err
	}
	p.current_tracks = uris
	p.cursor = 0
	p.startPolling(POLL_INTERVAL)
	return nil
}

func (p *SpotifyPlayer) Resume() (error) {
	err := p.actionWithRefresh(func() error {
		return p.client.Play()
	})

	// SPOTIFY RETURNS A SERVER ERROR IF THERE IS A TRACK CURRENTLY PLAYING
	if err != nil {
		return err
	}

	p.startPolling(POLL_INTERVAL)
	return nil
}

func (p *SpotifyPlayer) Pause() (error) {
	err := p.actionWithRefresh(func() error {
		return p.client.Pause()
	})

	if err != nil {
		return err
	}
	p.stopPolling()
	return nil
}

func (p *SpotifyPlayer) Next() (error) {
	err := p.actionWithRefresh(func() error {
		return p.client.Next()
	})

	if err != nil {
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

func (p *SpotifyPlayer) actionWithRefresh(action func() error) error {
	err := action()

	if err != nil {
		if spotify_error, ok := err.(*spotify.Error); ok {
			if spotify_error.Status == 401 && spotify_error.Message == "The access token expired" {
				if err := p.refreshClient(); err != nil {
					return err
				}
			} else {
				return err
			}
		} else {
			return err
		}

		return action()
	}

	return nil
}

func (p *SpotifyPlayer) refreshClient() (error) {
	// Hold lock to prevent other sessions from being 401'ed
	log.Println("LOCKING")
	p.refreshing.Lock()
	new_token, err := p.identity.GetToken(provider)
	if err != nil {
		p.refreshing.Unlock()
		return err
	}
	p.client = authenticator.NewClient(new_token)
	p.refreshing.Unlock()
	return nil
}

// Update the state of the players until a session becomes inactive
func (p *SpotifyPlayer) poll() {
	for {
		select {
		// Update states of our players
		case <-p.ticker.C:
			var state *spotify.PlayerState

			err := p.actionWithRefresh(func() error {
				inner_state, err := p.client.PlayerState()

				if err != nil {
					return err
				}

				state = inner_state
				return nil
			})

			if err != nil {
				log.Println("Failed getting spotify player state ", err)
				return
			}

			// TODO: Update player's state

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
