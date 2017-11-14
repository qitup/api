package spotify

import (
	"github.com/zmb3/spotify"
	"dubclan/api/party"
	"golang.org/x/oauth2"
)

var (
	Scopes = []string{"streaming", "user-library-read", "user-read-private", "user-read-playback-state", "user-modify-playback-state", "user-read-currently-playing"}

	authenticator = spotify.NewAuthenticator("", Scopes...)
)

type SpotifyPlayer struct {
	party.Player
	Client spotify.Client
	State  *spotify.PlayerState
}

func New(token *oauth2.Token) SpotifyPlayer {
	return SpotifyPlayer{
		Client: authenticator.NewClient(token),
		State:  nil,
	}
}

func (p *SpotifyPlayer) Play(item party.Item) (error) {
	panic("implement me")
}

func (p *SpotifyPlayer) Pause(item party.Item) (error) {
	panic("implement me")
}

func (p *SpotifyPlayer) Next(item party.Item) (error) {
	panic("implement me")
}

func (p *SpotifyPlayer) Previous(item party.Item) (error) {
	panic("implement me")
}

func (p *SpotifyPlayer) UpdateState() (error) {
	panic("implement me")
}
