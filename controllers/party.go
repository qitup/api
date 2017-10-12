package controllers

import (
	"github.com/gin-gonic/gin"
	"gopkg.in/mgo.v2"

	"dubclan/api/store"
	"dubclan/api/models"
	"gopkg.in/dgrijalva/jwt-go.v3"
	"github.com/zmb3/spotify"
	"os"
	"golang.org/x/oauth2"
	"time"
)

func CreateParty(context *gin.Context) {
	mongo := context.MustGet("mongo").(*mgo.Database)

	var data struct {
		Name     string `json:"name"`
		JoinCode string `json:"join_code"`
	}

	if context.BindJSON(&data) == nil {
		party := models.NewParty(data.JoinCode, data.Name, models.User{})

		store.InsertParty(mongo, party)

		context.JSON(200, party)
		return
	}

	context.Status(400)
}

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
