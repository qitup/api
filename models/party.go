package models

import (
	"time"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2"
	"github.com/garyburd/redigo/redis"
	"crypto/sha1"
	"errors"
	"encoding/base64"
)

const party_collection = "party"

var ConnectTokenIssued = errors.New("connect token currently issued for this user")

type Party struct {
	ID        bson.ObjectId `json:"id" bson:"_id"`
	HostID    bson.ObjectId `json:"host_id" bson:"host_id"`
	Attendees []*Attendee   `json:"attendees" bson:"attendees"`
	JoinCode  string        `json:"join_code" bson:"join_code"`
	Name      string        `json:"name" bson:"name"`
	CreatedAt time.Time     `json:"created_at" bson:"created_at"`
}

type Attendee struct {
	UserId   bson.ObjectId `json:"user_id" bson:"user_id"`
	JoinedAt time.Time     `json:"joined_at" bson:"joined_at"`
}

func NewParty(host bson.ObjectId, name, join_code string) (Party) {
	return Party{
		ID:        bson.NewObjectId(),
		HostID:    host,
		Attendees: []*Attendee{},
		Name:      name,
		JoinCode:  join_code,
		CreatedAt: time.Now(),
	}
}

func NewAttendee(user_id bson.ObjectId) Attendee {
	return Attendee{
		UserId:   user_id,
		JoinedAt: time.Now(),
	}
}

func PartyByCode(db *mgo.Database, code string) (*Party, error) {
	var party Party

	err := db.C(party_collection).Find(bson.M{"join_code": code}).One(&party)

	return &party, err
}

func (p *Party) Save(db *mgo.Database) error {
	_, err := db.C(party_collection).Upsert(bson.M{"_id": p.ID}, p)

	return err
}

func (p *Party) AddAttendee(db *mgo.Database, attendee *Attendee) (error) {
	err := db.C(party_collection).Update(bson.M{
		"_id": p.ID,
		"attendees": bson.M{
			"$not": bson.M{
				"$elemMatch": bson.M{
					"user_id": attendee.UserId,
				},
			},
		},
	}, bson.M{
		"$addToSet": bson.M{"attendees": attendee},
	})

	if err == nil {
		p.Attendees = append(p.Attendees, attendee)
	}

	return err
}

func (p *Party) InitiateConnect(redis redis.Conn, attendee *Attendee) (string, error) {
	token := attendee.UserId.Hex() + p.JoinCode
	hasher := sha1.New()
	hasher.Write([]byte(token))
	sha := base64.URLEncoding.EncodeToString(hasher.Sum(nil))

	if reply, err := redis.Do("GET", "jc:"+sha); err != nil {
		return "", err
	} else if reply == nil {
		if reply, err := redis.Do("SETEX", "jc:"+sha, 30, p.ID.Hex()); err != nil {
			return "", err
		} else if reply == "OK" {
			return sha, nil
		} else {
			return "", errors.New("failed setting connect token")
		}
	} else {
		return "", ConnectTokenIssued
	}
}
