package models

import (
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"time"
	"gopkg.in/dgrijalva/jwt-go.v3"
	"strings"
	"github.com/Pallinder/go-randomdata"
)

type APIClaims struct {
	jwt.StandardClaims
	Email    string `json:"email"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

const USER_COLLECTION = "users"

type User struct {
	ID         bson.ObjectId `json:"id" bson:"id"`
	Identities []*Identity   `json:"-" bson:"identities"`
	Email      string        `json:"email" bson:"email"`
	Username   string        `json:"username" bson:"username"`
	Name       string        `json:"name" bson:"name"`
	AvatarURL  string        `json:"avatar_url" bson:"avatar_url"`
}

type Users []User

type Identity struct {
	RawData           map[string]interface{} `json:"-" bson:"raw"`
	Provider          string                 `json:"provider" bson:"provider"`
	Email             string                 `json:"email" bson:"email"`
	Name              string                 `json:"name" bson:"name"`
	FirstName         string                 `json:"first_name,omitempty" bson:"first_name"`
	LastName          string                 `json:"last_name,omitempty" bson:"last_name"`
	NickName          string                 `json:"nick_name,omitempty" bson:"nick_name"`
	Description       string                 `json:"description,omitempty" bson:"description"`
	UserID            string                 `json:"user_id" bson:"user_id"`
	AvatarURL         string                 `json:"avatar_url,omitempty" bson:"avatar_url"`
	Location          string                 `json:"location,omitempty" bson:"location"`
	AccessToken       string                 `json:"-" bson:"access_token"`
	AccessTokenSecret string                 `json:"-" bson:"access_token_secret"`
	RefreshToken      string                 `json:"-" bson:"refresh_token"`
	ExpiresAt         time.Time              `json:"expires_at,omitempty" bson:"expires_at"`
}

func (u *User) Save(db *mgo.Database) error {
	_, err := db.C(USER_COLLECTION).Upsert(bson.M{"_id": u.ID}, u)
	return err
}

func UserByID(db *mgo.Database, id bson.ObjectId) (User, error) {
	var user User
	err := db.C(USER_COLLECTION).FindId(id).One(&user)

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

	_, err := db.C(USER_COLLECTION).Find(bson.M{
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
	bulk := db.C(USER_COLLECTION).Bulk()
	bulk.Update(
		bson.M{"_id": id},
		bson.M{"$pull": bson.M{"identities.email": identity.Email, "identities.provider": identity.Provider}},
		bson.M{"_id": id},
		bson.M{"$addToSet": bson.M{"identities": identity}},
	)

	_, err := bulk.Run()

	return err
}

func (u *User) NewToken(host string, signing_key []byte) (string, error) {
	claims := APIClaims{
		StandardClaims: jwt.StandardClaims{
			IssuedAt:  time.Now().Unix(),
			ExpiresAt: time.Now().Add(time.Hour * 6).Unix(),
			Issuer:    host,
			Subject:   u.ID.Hex(),
			Audience:  "qitup-app",
		},
		Email:    u.Email,
		Name:     u.Name,
		Username: u.Username,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign and get the complete encoded token as a string using the secret
	return token.SignedString(signing_key)
}

func (u *User) AssumeIdentity(db *mgo.Database, identity Identity) error {
	if len(strings.TrimSpace(identity.NickName)) != 0 {
		u.Username = identity.NickName
	} else if len(strings.TrimSpace(identity.UserID)) != 0 {
		u.Username = identity.UserID
	} else {
		u.Username = randomdata.SillyName()
	}

	if len(strings.TrimSpace(identity.Name)) != 0 {
		u.Name = identity.Name
	}

	if len(strings.TrimSpace(identity.AvatarURL)) != 0 {
		u.AvatarURL = identity.AvatarURL
	} else {
		u.AvatarURL = "https://api.adorable.io/avatars/" + u.Username
	}

	u.Email = identity.Email

	return u.Save(db)
}

func (u *User) GetIdentity(provider string) *Identity {
	for _, identity := range u.Identities {
		if identity.Provider == provider {
			return identity
		}
	}

	return nil
}
