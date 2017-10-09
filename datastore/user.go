package datastore

import (
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"dubclan/api/models"
)

type User struct {
	Me *models.User
}

func NewUser(db *mgo.Database, u *models.User) *User {
	return &User{Me: u}
}

func (u *User) Save(db *mgo.Database, p *models.Party) error {
	_, err := coll(db).Upsert(bson.M{"_id": u.Me.ID}, u.Me)
	return err
}
//
//func FindByID(id bson.ObjectId, db *mgo.Database) (models.Party, error) {
//	var party models.User
//	err := coll(db).FindId(id).One(party)
//
//	return party, err
//}

//func coll(db *mgo.Database) *mgo.Collection {
//	return db.C("user")
//}
