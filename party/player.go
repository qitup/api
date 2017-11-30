//go:generate stringer -type=EventType player.go
package party

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
}

type EventType int

const (
	PLAYBACK_START       EventType = iota
	PLAYBACK_INTERRUPTED
	QUEUE_CHANGED
)

type Event struct {
	Type     EventType              `json:"type"`
}

type PlaybackStart struct {
	Event
}

type PlaybackPause struct {
	Event
}

type PlaybackInterrupted struct {
	Event
}

type QueueChanged struct {
	Event
	Queue Queue	`json:"queue"`
}