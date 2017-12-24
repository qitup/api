package models

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/zmb3/spotify"
	"gopkg.in/mgo.v2/bson"
)

type Item interface {
	Added(by bson.ObjectId)
	UpdateState(state ItemState)
	GetType() (string)
	GetPlayerType() (string)
	Play() (bool)
	Pause() (bool)
	Done() (bool)
}

type ItemState struct {
	Progress  int  `json:"progress"`
	Playing   bool `json:"playing"`
	Completed bool `json:"completed"`
}

type BaseItem struct {
	Item                  `json:"-"`
	Type    string        `json:"type" bson:"type"`
	AddedBy bson.ObjectId `json:"added_by" bson:"added_by,omitempty"`
	AddedAt time.Time     `json:"added_at" bson:"added_at"`
	State   ItemState     `json:"state" bson:"state"`
}

func (i *BaseItem) Added(by bson.ObjectId) {
	i.AddedAt = time.Now()
	i.AddedBy = by
}

func (i *BaseItem) GetType() string {
	return i.Type
}

func (i *BaseItem) Play() bool {
	if i.State.Playing {
		return false
	}

	i.State.Playing = true
	return true
}

func (i *BaseItem) Pause() bool {
	if !i.State.Playing {
		return false
	}

	i.State.Playing = false
	return true
}

func (i *BaseItem) Done() bool {
	if i.State.Completed {
		return false
	}

	i.State.Completed = true
	return true
}

type ItemUnpacker struct {
	Result Item
}

func (u *ItemUnpacker) UnmarshalJSON(b []byte) (error) {
	var m map[string]interface{}

	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}

	if itemType, ok := m["type"].(string); ok {
		var item Item
		switch itemType {
		case "spotify_track":
			item = &SpotifyTrack{}
			break
		default:
			return errors.New("invalid item type")
		}

		if err := json.Unmarshal(b, item); err != nil {
			return err
		}

		u.Result = item
		return nil
	}

	return errors.New("item missing type field")
}

type SpotifyTrack struct {
	BaseItem
	URI spotify.URI `json:"uri" bson:"uri"`
}

func (i *SpotifyTrack) GetType() string {
	return i.Type
}

func (i *SpotifyTrack) GetPlayerType() (string) {
	return "spotify"
}
