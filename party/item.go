package party

import (
	"time"
	"errors"
	"encoding/json"
	"gopkg.in/mgo.v2/bson"
	"github.com/zmb3/spotify"
)

type Item interface {
	Added(by bson.ObjectId)
}

type BaseItem struct {
	Type    string        `json:"type" bson:"type"`
	AddedBy bson.ObjectId `json:"added_by" bson:"added_by"`
	AddedAt time.Time     `json:"added_at" bson:"added_at"`
}

func (i *BaseItem) Added(by bson.ObjectId) {
	i.AddedAt = time.Now()
	i.AddedBy = by
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
