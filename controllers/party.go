package controllers

import (
	"github.com/gin-gonic/gin"
	"gopkg.in/dgrijalva/jwt-go.v3"
	"github.com/zmb3/spotify"
	"os"
	"golang.org/x/oauth2"
	"time"
	"dubclan/api/models"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2"
	"crypto/sha1"
	"encoding/base64"
	"github.com/garyburd/redigo/redis"
	"github.com/urfave/cli"
	"errors"
	"net/url"
	"log"
)

func CreateParty(context *gin.Context) {
	mongo := context.MustGet("mongo").(*mgo.Database)

	var data struct {
		Name     string `json:"name"`
		JoinCode string `json:"join_code"`
	}

	user_id := bson.ObjectIdHex(context.MustGet("userID").(string))

	if context.BindJSON(&data) == nil {
		party := models.NewParty(user_id, data.Name, data.JoinCode)

		log.Printf("%v", party)

		if err := party.Save(mongo); err != nil {
			context.Error(err)
			return
		}

		context.JSON(200, party)
		return
	}

	context.Status(400)
}

// Create unique join url for user
//		join_token: sha2(user_id + join_code)
// set key in redis with 30 sec ttl
// 		SETEX join_token 30
func JoinParty(redis redis.Conn, context *gin.Context, cli *cli.Context) {
	mongo := context.MustGet("mongo").(*mgo.Database)

	code := context.Query("code")

	party, err := models.PartyByCode(mongo, code)

	if err != nil {
		context.Error(err)
	}

	join_code := context.GetString("userID") + code
	hasher := sha1.New()
	hasher.Write([]byte(join_code))
	sha := base64.URLEncoding.EncodeToString(hasher.Sum(nil))

	if reply, err := redis.Do("GET", "jc:"+sha); err != nil {
		context.AbortWithError(500, err)
	} else if reply == nil {
		if reply, err := redis.Do("SETEX", "jc:"+sha, 30, 1); err != nil {
			context.AbortWithError(500, err)
		} else if reply == "OK" {

			context.JSON(200, gin.H{
				"url":   cli.String("public-ws-host") + "/party/connect/" + url.PathEscape(sha),
				"party": party,
			})
		} else {
			context.AbortWithError(500, errors.New("failed setting connect url"))
		}
	} else {
		context.Status(204)
	}
}

//func ConnectParty

func GetParty(c *gin.Context) {
	claims := c.MustGet("JWT_PAYLOAD").(jwt.MapClaims)

	auth := spotify.NewAuthenticator(os.Getenv("BASE_HOST")+"/auth/spotify/callback", spotify.ScopeUserReadPrivate)

	client := auth.NewClient(&oauth2.Token{
		AccessToken: claims["access_token"].(string),
		Expiry:      time.Unix(int64(claims["exp"].(float64)), 0),
	})

	client.PlayOpt(&spotify.PlayOptions{
		URIs: []spotify.URI{"spotify:track:29tzM8oIgOxBr3cI9CBOpb"},
	})
}
