package store

import (
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"dubclan/api/models"
)

const user_collection = "user"

func Save(db *mgo.Database, u *models.User) error {
	_, err := db.C(user_collection).Upsert(bson.M{"_id": u.ID}, u)
	return err
}

func UserByID(id bson.ObjectId, db *mgo.Database) (models.User, error) {
	var user models.User
	err := db.C(user_collection).FindId(id).One(user)

	return user, err
}

func UserByIdentity(identity models.Identity, db *mgo.Database) (models.User, error) {
	var user models.User

	err := db.C(user_collection).Find(bson.M{
		"identities": bson.M{
			"$elemMatch": bson.M{
				"email":    identity.Email,
				"provider": identity.Provider,
			},
		},
	}).One(user)

	return user, err
}
