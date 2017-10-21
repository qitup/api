package models

import (
	"time"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2"
)

const party_collection = "party"

type Party struct {
	ID          bson.ObjectId   `json:"id" bson:"_id"`
	HostID      bson.ObjectId   `json:"-" bson:"host_id"`
	AttendeeIDs []bson.ObjectId `json:"-" bson:"attendee_ids"`
	JoinCode    string          `json:"join_code" bson:"join_code"`
	Name        string          `json:"name" bson:"name"`
	CreatedAt   time.Time       `json:"created_at" bson:"created_at"`
}

func NewParty(host bson.ObjectId, name, join_code string) (Party) {
	return Party{
		ID:          bson.NewObjectId(),
		HostID:      host,
		AttendeeIDs: []bson.ObjectId{},
		Name:        name,
		JoinCode:    join_code,
		CreatedAt:   time.Now(),
	}
}

func (p *Party) Save(db *mgo.Database) error {
	_, err := db.C(party_collection).Upsert(bson.M{"_id": p.ID}, p)

	return err
}

func PartyByCode(db *mgo.Database, code string) (*Party, error) {
	var party Party

	err := db.C(party_collection).Find(bson.M{"join_code": code}).One(&party)

	return &party, err
}
