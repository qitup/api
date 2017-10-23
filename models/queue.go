package models

import (
	"github.com/zmb3/spotify"
	"github.com/garyburd/redigo/redis"
	"encoding/json"
)

type Queue struct {
	Items []*BaseItem `json:"items" bson:"items"`
}

func NewQueue() *Queue {
	return &Queue{
		Items: []*BaseItem{},
	}
}

func (q *Queue) Push(redis redis.Conn, party string, item *BaseItem) error {
	if serialized, err := item.Serialize(); err == nil {
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

type ItemType interface {
	Generic() (interface{})
	Serialize() (string, error)
}

type BaseItem struct {
	baseitem ItemType
}

func (i *BaseItem) Serialize() (string, error) {
	return i.baseitem.Serialize()
}

func (i *BaseItem) Generic() (interface{}) {
	return i.baseitem.Generic()
}

//func (i *BaseItem) Deserialize() (BaseItem, error) {
//	serialized, err := json.Unmarshal(i)
//
//	return string(serialized), err
//}

type SpotifyTrack struct {
	*BaseItem
	URI spotify.URI `json:"uri" bson:"uri"`
}

func NewSpotifyTrack(uri spotify.URI) *SpotifyTrack {
	track := &SpotifyTrack{&BaseItem{nil}, uri}
	track.baseitem = track

	return track
}

func (i *SpotifyTrack) Serialize() (string, error) {
	serialized, err := json.Marshal(i)

	return string(serialized), err
}

func (i *SpotifyTrack) Generic() (interface{}) {
	return i
}
