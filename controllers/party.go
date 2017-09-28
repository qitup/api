package controllers

import (
	"github.com/gin-gonic/gin"
	"gopkg.in/mgo.v2"

	"dubclan/api/datastore"
	"dubclan/api/models"
)

func CreateParty(context *gin.Context) {
	session := context.MustGet("session").(*mgo.Session)

	var data struct {
		Name     string `json:"name" bson:"name"`
		JoinCode string `json:"join_code" bson:"join_code"`
	}

	if context.BindJSON(&data) == nil {
		party := models.NewParty(data.JoinCode, data.Name)

		datastore.InsertParty(session.DB("test"), party)

		context.JSON(200, party)
		return
	}

	context.Status(400)
}

func GetParty(c *gin.Context) {

}
