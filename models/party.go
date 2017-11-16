package models

import (
	"time"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2"
	"errors"
)

const PARTY_COLLECTION = "parties"

type Party struct {
	ID        bson.ObjectId `json:"id" bson:"_id"`
	HostID    bson.ObjectId `json:"-" bson:"host_id"`
	Host      *User          `json:"host" bson:"host,omitempty"`
	Attendees []*Attendee   `json:"attendees" bson:"attendees"`
	JoinCode  string        `json:"join_code" bson:"join_code"`
	Name      string        `json:"name" bson:"name"`
	CreatedAt time.Time     `json:"created_at" bson:"created_at"`
}

type Attendee struct {
	UserId   bson.ObjectId `json:"-" bson:"user_id"`
	User     User          `json:"user" bson:"user,omitempty"`
	JoinedAt time.Time     `json:"joined_at" bson:"joined_at"`
}

func NewParty(host_id bson.ObjectId, name, join_code string) (Party) {
	return Party{
		ID:        bson.NewObjectId(),
		HostID:    host_id,
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

	err := db.C(PARTY_COLLECTION).Pipe([]bson.M{
		{"$match": bson.M{"join_code": code}},
		{
			"$lookup": bson.M{
				"localField":   "host_id",
				"from":         USER_COLLECTION,
				"foreignField": "_id",
				"as":           "host",
			},
		},
		{"$unwind": "$host"},
		{"$unwind": bson.M{"path": "$attendees", "preserveNullAndEmptyArrays": true}},
		{
			"$lookup": bson.M{
				"localField":   "attendees.user_id",
				"from":         USER_COLLECTION,
				"foreignField": "_id",
				"as":           "attendees.user",
			},
		},
		{"$unwind": bson.M{"path": "$attendees.user", "preserveNullAndEmptyArrays": true}},
		{
			"$group": bson.M{
				"_id":        "$_id",
				"name":       bson.M{"$first": "$name"},
				"join_code":  bson.M{"$first": "$join_code"},
				"created_at": bson.M{"$first": "$created_at"},
				"host_id":    bson.M{"$first": "$host_id"},
				"host":       bson.M{"$first": "$host"},
				"attendees":  bson.M{"$push": "$attendees"},
			},
		},
		{
			"$project": bson.M{
				"name":       1,
				"join_code":  1,
				"created_at": 1,
				"host_id":    1,
				"host":       1,
				"attendees": bson.M{
					"$cond": []interface{}{bson.M{"$ne": []interface{}{"$attendees.user", []interface{}{}}}, "$attendees", []interface{}{}},
				},
			},
		},
	}).One(&party)

	return &party, err
}

func PartyByID(db *mgo.Database, id bson.ObjectId) (*Party, error) {
	var party Party

	err := db.C(PARTY_COLLECTION).Pipe([]bson.M{
		{"$match": bson.M{"_id": id}},
		{
			"$lookup": bson.M{
				"localField":   "host_id",
				"from":         USER_COLLECTION,
				"foreignField": "_id",
				"as":           "host",
			},
		},
		{"$unwind": "$host"},
		{"$unwind": bson.M{"path": "$attendees", "preserveNullAndEmptyArrays": true}},
		{
			"$lookup": bson.M{
				"localField":   "attendees.user_id",
				"from":         USER_COLLECTION,
				"foreignField": "_id",
				"as":           "attendees.user",
			},
		},
		{"$unwind": bson.M{"path": "$attendees.user", "preserveNullAndEmptyArrays": true}},
		{
			"$group": bson.M{
				"_id":        "$_id",
				"name":       bson.M{"$first": "$name"},
				"join_code":  bson.M{"$first": "$join_code"},
				"created_at": bson.M{"$first": "$created_at"},
				"host_id":    bson.M{"$first": "$host_id"},
				"host":       bson.M{"$first": "$host"},
				"attendees":  bson.M{"$push": "$attendees"},
			},
		},
		{
			"$project": bson.M{
				"name":       1,
				"join_code":  1,
				"created_at": 1,
				"host_id":    1,
				"host":       1,
				"attendees": bson.M{
					"$cond": []interface{}{bson.M{"$ne": []interface{}{"$attendees.user", []interface{}{}}}, "$attendees", []interface{}{}},
				},
			},
		},
	}).One(&party)

	return &party, err
}

func (p *Party) Insert(db *mgo.Database) error {
	err := db.C(PARTY_COLLECTION).Insert(p)

	return err
}

func (p *Party) Save(db *mgo.Database) error {
	_, err := db.C(PARTY_COLLECTION).Upsert(bson.M{"_id": p.ID}, p)

	return err
}

func (p *Party) AddAttendee(db *mgo.Database, attendee *Attendee) (error) {
	err := db.C(PARTY_COLLECTION).Update(bson.M{
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

func (p *Party) WithHost(db *mgo.Database) (error) {
	if p.HostID.Valid() {
		if user, err := UserByID(db, p.HostID); err == nil {
			p.Host = user
			return nil
		} else {
			return err
		}
	} else {
		return errors.New("invalid party host")
	}
}
