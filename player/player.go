package player

import (
	"dubclan/api/models"
)

type Player interface {
	Play(item []models.Item) (error)
	Pause() (error)
	Resume() (error)
	Next() (error)
	Previous() (error)
	HasItems() (bool)
	Stop()
	GetState() (int)
}

type EventType int

const (
	READY       = iota
	PLAYING
	PAUSED
	INTERRUPTED
)
