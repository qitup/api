package models

import (
	"github.com/zmb3/spotify"
	"github.com/garyburd/redigo/redis"
	"encoding/json"
)

type Queue struct {
	Items []*BaseItem `json:"items" bson:"items"`
}

type BaseItem struct {
	ItemType string `json:"item_type" bson:"item_type"`
}

func (i *BaseItem) Serialize() (string, error) {
	serialized, err := json.Marshal(i)

	return string(serialized), err
}

//func (i *BaseItem) Deserialize() (BaseItem, error) {
//	serialized, err := json.Unmarshal(i)
//
//	return string(serialized), err
//}

type SpotifyTrack struct {
	BaseItem
	spotify.URI `json:"uri" bson:"uri"`
}

func (q *Queue) Push(redis redis.Conn, party string, item *BaseItem) error {
	if serialized, err := item.Serialize(); err == nil {
		if _, err := redis.Do("LPUSH", party, serialized); err == nil {
			return nil
		} else {
			return err
		}
	} else {
		return nil
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