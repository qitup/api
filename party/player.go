package party

import (
	"golang.org/x/oauth2"
)

type Player interface {
	New(token *oauth2.Token)
	Play(item Item) (error)
	Pause(item Item) (error)
	Next(item Item) (error)
	Previous(item Item) (error)
	UpdateState() (error)
}

type Event struct {
	Type     string                 `json:"type"`
	Name     string                 `json:"name"`
	Metadata map[string]interface{} `json:"metadata"`
}
