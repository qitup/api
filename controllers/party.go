package controllers

import (
	"github.com/gin-gonic/gin"
	"gopkg.in/mgo.v2"

	"dubclan/api/datastore"
	"dubclan/api/models"
	"github.com/dgrijalva/jwt-go"
	"github.com/zmb3/spotify"
	"os"
	"golang.org/x/oauth2"
	"time"
)

func CreateParty(context *gin.Context) {
	mongo_session := context.MustGet("mongo_session").(*mgo.Session)

	var data struct {
		Name     string `json:"name"`
		JoinCode string `json:"join_code"`
	}

	if context.BindJSON(&data) == nil {
		party := models.NewParty(data.JoinCode, data.Name, models.User{})

		datastore.InsertParty(mongo_session.DB("test"), party)

		context.JSON(200, party)
		return
	}

	context.Status(400)
}

func GetParty(c *gin.Context) {
	claims := c.Request.Context().Value("user").(*jwt.Token).Claims.(jwt.MapClaims)

	auth := spotify.NewAuthenticator(os.Getenv("BASE_HOST")+"/auth/spotify/callback", spotify.ScopeUserReadPrivate)

	client := auth.NewClient(&oauth2.Token{
		AccessToken: claims["access_token"].(string),
		Expiry:      time.Unix(int64(claims["exp"].(float64)), 0),
	})

	client.PlayOpt(&spotify.PlayOptions{
		URIs: []spotify.URI{"spotify:track:29tzM8oIgOxBr3cI9CBOpb"},
	})
}
