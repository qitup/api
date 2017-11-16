package models

import (
	"time"
	"errors"
	"encoding/json"
	"gopkg.in/mgo.v2/bson"
	"github.com/zmb3/spotify"
)

type Item interface {
	Added(by bson.ObjectId)
	UpdateState(state ItemState)
	GetType()string
}

type ItemState struct {
	Progress int `json:"progress"`
}

type BaseItem struct {
	Item
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

func (i *BaseItem) UpdateState(new ItemState) {
	i.State = new
}

func UnmarshalItem(b []byte) (Item, error) {
	var m map[string]interface{}

	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}

	if item_type, ok := m["type"].(string); ok {
		var item Item
		switch item_type {
		case "spotify_track":
			item = &SpotifyTrack{}
			break
		default:
			return nil, errors.New("invalid item type")
		}

		if err := json.Unmarshal(b, item); err != nil {
			return nil, err
		}

		return item, nil
	}

	return nil, errors.New("item missing type field")
}

type SpotifyTrack struct {
	BaseItem
	URI spotify.URI `json:"uri" bson:"uri"`
}

func (i *SpotifyTrack) Added(by bson.ObjectId) {
	i.AddedAt = time.Now()
	i.AddedBy = by
}

func (i *SpotifyTrack) GetType() string {
	return i.Type
}

func (i *SpotifyTrack) UpdateState(new ItemState) {
	i.State = new
}