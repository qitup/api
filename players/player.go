package player

import "dubclan/api/party"

type Player interface {
	Play(item party.Item) (error)
	Stop(item party.Item) (error)
	Next(item party.Item) (error)
	Previous(item party.Item) (error)
}

type Event struct {
	Type     string                 `json:"type"`
	Name     string                 `json:"name"`
	Metadata map[string]interface{} `json:"metadata"`
}
