package models

import (
	"time"
	"gopkg.in/mgo.v2/bson"
)

type Party struct {
	ID        bson.ObjectId `json:"id" bson:"_id"`
	Host      User          `json:"host" bson:"host"`
	JoinCode  string        `json:"join_code" bson:"join_code"`
	Name      string        `json:"name" bson:"name"`
	Attendees Users         `json:"attendees" bson:"attendees"`
	CreatedAt time.Time     `json:"created_at" bson:"created_at"`
}

func NewParty(join_code, name string, host User) *Party {
	return &Party{
		ID:        bson.NewObjectId(),
		Host:      host,
		JoinCode:  join_code,
		Name:      name,
		Attendees: Users{},
		CreatedAt: time.Now(),
	}
}
