package models

import "gopkg.in/mgo.v2/bson"

type User struct {
	ID         bson.ObjectId `json:"id" bson:"id"`
	Identities []Identity    `json:"identities" bson:"identities"`
	Username   string        `json:"username" bson:"username"`
	FirstName  string
	LastName   string
	Email      string
}

type Users []User
