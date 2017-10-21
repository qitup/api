package models

import (
	"gopkg.in/mgo.v2/bson"
)

type User struct {
	ID         bson.ObjectId `json:"id" bson:"id"`
	Identities []*Identity   `json:"identities,omitempty" bson:"identities"`
	Username   string        `json:"username" bson:"username"`
}

type Users []User
