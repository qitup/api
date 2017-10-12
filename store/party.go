package store

import (
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"dubclan/api/models"
)

const collection = "users"

func InsertParty(db *mgo.Database, p *models.Party) error {
	err := coll(db).Insert(p)
	return err
}

func FindByID(id bson.ObjectId, db *mgo.Database) (models.Party, error) {
	var party models.Party
	err := coll(db).FindId(id).One(party)

	return party, err
}

func coll(db *mgo.Database) *mgo.Collection {
	return db.C("party")
}
