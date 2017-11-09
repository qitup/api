package player

import (
	"dubclan/api/party"
	"golang.org/x/oauth2"
)

type Player interface {
	New(token *oauth2.Token)
	Play(item party.Item) (error)
	Stop(item party.Item) (error)
	Next(item party.Item) (error)
	Previous(item party.Item) (error)
	UpdateState() (error)
}

type Event struct {
	Type     string                 `json:"type"`
	Name     string                 `json:"name"`
	Metadata map[string]interface{} `json:"metadata"`
}
