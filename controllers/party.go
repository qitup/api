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
	"github.com/garyburd/redigo/redis"
	"github.com/urfave/cli"
	"net/url"
)

func CreateParty(redis redis.Conn, context *gin.Context, cli *cli.Context) {
	mongo := context.MustGet("mongo").(*mgo.Database)

	var data struct {
		Name     string `json:"name"`
		JoinCode string `json:"join_code"`
	}

	user_id := bson.ObjectIdHex(context.MustGet("userID").(string))

	if context.BindJSON(&data) == nil {
		party := models.NewParty(user_id, data.Name, data.JoinCode)

		if err := party.Save(mongo); err != nil {
			context.AbortWithError(500, err)
			return
		}

		me := models.User{
			ID: bson.ObjectIdHex(context.GetString("userID")),
		}

		connect_token, err := party.InitiateConnect(redis, &me)

		switch err {
		case nil:
			context.JSON(200, gin.H{
				"url":   cli.String("public-ws-host") + "/party/connect/" + url.PathEscape(connect_token),
				"party": party,
			})
			return
		case models.ConnectTokenIssued:
			context.JSON(403, gin.H{
				"error": gin.H{
					"code": "already_issued",
					"msg":  "Connect token already issued",
				}})
			return
		default:
			context.AbortWithError(500, err)
			return
		}
	} else {
		context.Status(400)
	}
}

// Create unique join url for user
//		join_token: sha2(user_id + join_code)
// set key in redis with 30 sec ttl
// 		SETEX join_token 30
func JoinParty(redis redis.Conn, context *gin.Context, cli *cli.Context) {
	mongo := context.MustGet("mongo").(*mgo.Database)

	code := context.Query("code")

	party, err := models.PartyByCode(mongo, code)

	if err == mgo.ErrNotFound {
		context.JSON(400, gin.H{
			"error": gin.H{
				"code": "party_not_found",
				"msg":  "Party not found",
			},
		})
		return
	} else if err != nil {
		context.AbortWithError(500, err)
		return
	}

	me := models.User{
		ID: bson.ObjectIdHex(context.GetString("userID")),
	}

	connect_token, err := party.InitiateConnect(redis, &me)

	if me.ID != party.HostID {
		if err := party.AddAttendee(mongo, &me); err != nil {
			context.Error(err)
		}
	}

	switch err {
	case nil:
		context.JSON(200, gin.H{
			"url":   cli.String("public-ws-host") + "/party/connect/" + url.PathEscape(connect_token),
			"party": party,
		})
		break
	case models.ConnectTokenIssued:
		context.JSON(403, gin.H{
			"error": gin.H{
				"code": "already_issued",
				"msg":  "Connect token already issued",
			}})
		break
	default:
		context.AbortWithError(500, err)
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
