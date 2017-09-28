package models

import "gopkg.in/mgo.v2/bson"

type User struct {
	ID   bson.ObjectId `bson:"id"`
	Name string        `bson:"name"`
}

type Users []*User