package main

import (
	"github.com/gin-gonic/gin"
	"gopkg.in/mgo.v2"

	"dubclan/api/middleware"
	"dubclan/api/controllers"
)

func main() {
	// Disable Console Color
	// gin.DisableConsoleColor()
	r := gin.Default()

	// Ping test
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})

	session, err := mgo.Dial("mongodb://mongodb:27017")

	if err != nil {
		panic(err)
	}
	defer session.Close()

	r.Use(middleware.Store(session))

	party := r.Group("/party")

	party.GET("/:join_code", controllers.GetParty)
	party.POST("", controllers.CreateParty)

	// Listen and Server in 0.0.0.0:8080
	r.Run(":8080")
}
