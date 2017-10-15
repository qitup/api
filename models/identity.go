package models

import "time"

type Identity struct {
	RawData           map[string]interface{} `bson:"raw"`
	Provider          string                 `bson:"provider"`
	Email             string                 `bson:"email"`
	Name              string                 `bson:"name"`
	FirstName         string                 `bson:"first_name"`
	LastName          string                 `bson:"last_name"`
	NickName          string                 `bson:"nick_name"`
	Description       string                 `bson:"description"`
	UserID            string                 `bson:"user_id"`
	AvatarURL         string                 `bson:"avatar_url"`
	Location          string                 `bson:"location"`
	AccessToken       string                 `bson:"access_token"`
	AccessTokenSecret string                 `bson:"access_token_secret"`
	RefreshToken      string                 `bson:"refresh_token"`
	ExpiresAt         time.Time              `bson:"expires_at"`
}
