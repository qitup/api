package models

import (
	"github.com/zmb3/spotify"
	"github.com/garyburd/redigo/redis"
	"encoding/json"
	"gopkg.in/mgo.v2/bson"
	"errors"
)

type Queue struct {
	Items []*BaseItem `json:"items" bson:"items"`
}

func NewQueue() *Queue {
	return &Queue{
		Items: []*BaseItem{},
	}
}

func (q *Queue) Push(redis redis.Conn, party string, item BaseItem) error {
	if serialized, err := json.Marshal(item); err == nil {
		_, err := redis.Do("LPUSH", party, serialized)
		return err
	} else {
		return err
	}
}

//func (q *Queue) Pop(redis redis.Conn, party string, item *BaseItem) error {
//	if item, err := redis.Do("RPOP", party); err == nil {
//		if serialized, err := item.Deserialize(); err == nil {
//
//		} else {
//			return nil
//		}
//	} else {
//		return err
//	}
//}

type Item interface {
}

type BaseItem struct {
	Item    Item          `json:"item" bson:"item"`
	AddedBy bson.ObjectId `json:"added_by" bson:"added_by"`
}

func (i *BaseItem) UnmarshalJSON(b []byte) error {
	var m map[string]string

	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}

	switch m["type"] {
	case "spotify_track":
		var item SpotifyTrack
		if err := json.Unmarshal(b, &item); err != nil {
			return err
		}
		i.Item = &item
		break
	default:
		return errors.New("invalid item type")
	}

	return nil
}

type SpotifyTrack struct {
	URI spotify.URI `json:"uri" bson:"uri"`
}

func (t *SpotifyTrack) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{
		"type": "spotify_track",
		"uri":  string(t.URI),
	})
}
