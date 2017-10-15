package store

import (
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"dubclan/api/models"
	"log"
)

const user_collection = "user"

func SaveUser(db *mgo.Database, u *models.User) error {
	_, err := db.C(user_collection).Upsert(bson.M{"_id": u.ID}, u)
	return err
}

func UserByID(db *mgo.Database, id bson.ObjectId) (models.User, error) {
	var user models.User
	err := db.C(user_collection).FindId(id).One(&user)

	return user, err
}

func UpdateUserByIdentity(db *mgo.Database, identity models.Identity) (*models.User, error) {
	var user models.User

	change := mgo.Change{
		Update: bson.M{
			"$set": bson.M{
				"identities.$": identity,
			},
		},
		ReturnNew: true,
	}

	log.Println(change)

	_, err := db.C(user_collection).Find(bson.M{
		"identities": bson.M{
			"$elemMatch": bson.M{
				"email":    identity.Email,
				"provider": identity.Provider,
			},
		},
	}).Apply(change, &user)

	return &user, err
}

func UpdateIdentityById(db *mgo.Database, id bson.ObjectId, identity models.Identity) (error) {
	bulk := db.C(user_collection).Bulk()
	bulk.Update(
		bson.M{"_id": id},
		bson.M{"$pull": bson.M{"identities.email": identity.Email, "identities.provider": identity.Provider}},
		bson.M{"_id": id},
		bson.M{"$addToSet": bson.M{"identities": identity}},
	)

	_, err := bulk.Run()

	return err
}
