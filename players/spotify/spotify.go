package spotify

import (
	"github.com/zmb3/spotify"
	"dubclan/api/party"
	"golang.org/x/oauth2"
)

var (
	scopes = []string{"streaming", "user-library-read", "user-read-private", "user-read-playback-state", "user-modify-playback-state", "user-read-currently-playing"}

	authenticator = spotify.NewAuthenticator("", scopes...)
)

type SpotifyPlayer struct {
	Client spotify.Client
	State spotify.PlayerState
}

func New(token *oauth2.Token) SpotifyPlayer {
	return SpotifyPlayer{
		Client: authenticator.NewClient(token),
		State: spotify.PlayerState{},
	}
}

func (p *SpotifyPlayer) Play(item party.SpotifyTrack) (error) {
	panic("implement me")
}

func (p *SpotifyPlayer) Stop(item party.SpotifyTrack) (error) {
	panic("implement me")
}

func (p *SpotifyPlayer) Next(item party.SpotifyTrack) (error) {
	panic("implement me")
}

func (p *SpotifyPlayer) Previous(item party.SpotifyTrack) (error) {
	panic("implement me")
}

