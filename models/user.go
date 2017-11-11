package models

import (
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"time"
	"gopkg.in/dgrijalva/jwt-go.v3"
)

type APIClaims struct {
	jwt.StandardClaims
	AccessToken string `json:"access_token"`
}

const user_collection = "user"

type User struct {
	ID         bson.ObjectId `json:"id" bson:"id"`
	Identities []*Identity   `json:"-" bson:"identities"`
	Username   string        `json:"username" bson:"username"`
}

type Users []User

func SaveUser(db *mgo.Database, u *User) error {
	_, err := db.C(user_collection).Upsert(bson.M{"_id": u.ID}, u)
	return err
}

func UserByID(db *mgo.Database, id bson.ObjectId) (User, error) {
	var user User
	err := db.C(user_collection).FindId(id).One(&user)

	return user, err
}

func UpdateUserByIdentity(db *mgo.Database, identity Identity) (*User, error) {
	var user User

	change := mgo.Change{
		Update: bson.M{
			"$set": bson.M{
				"identities.$": identity,
			},
		},
		ReturnNew: true,
	}

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

func UpdateIdentityById(db *mgo.Database, id bson.ObjectId, identity Identity) (error) {
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

func (u *User) NewToken(signing_key []byte) (string, error) {
	claims := APIClaims{
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: int64(time.Hour * 24),
			Issuer:    "qitup.ca",
			Subject:   u.ID.Hex(),
		},
	}

	for _, identity := range u.Identities {
		if identity.Provider == "spotify" {
			claims.AccessToken = identity.AccessToken
		}
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign and get the complete encoded token as a string using the secret
	return token.SignedString(signing_key)
}