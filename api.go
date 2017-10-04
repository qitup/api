package main

import (
	"github.com/gin-gonic/gin"
	"gopkg.in/mgo.v2"

	"dubclan/api/middleware"
	"dubclan/api/controllers"
	"net/http"
	"fmt"
	//"github.com/zmb3/spotify"
	"github.com/markbates/goth/gothic"
	provider_spotify "github.com/markbates/goth/providers/spotify"
	"github.com/markbates/goth"
	"os"
	"gopkg.in/boj/redistore.v1"
	"strings"
	"regexp"
)

func main() {
	// Disable Console Color
	// gin.DisableConsoleColor()
	r := gin.Default()

	// Ping test
	r.GET("/ping", func(context *gin.Context) {
		user := context.MustGet("user").(goth.User)
		context.JSON(200, user)
	})

	// Fetch new store.
	store, err := redistore.NewRediStore(10, "tcp", "redis:6379", "", []byte("secret-key"))
	if err != nil {
		panic(err)
	}
	defer store.Close()

	gothic.Store = store

	goth.UseProviders(
		provider_spotify.New(os.Getenv("SPOTIFY_ID"), os.Getenv("SPOTIFY_SECRET"), "http://localhost:8081/auth/spotify/callback"),
	)

	gothic.GetProviderName = func(req *http.Request) (string, error) {
		parts := strings.Split(req.URL.Path, "/")

		return parts[2], nil
	}

	r.GET("/auth/:provider/callback", func(context *gin.Context) {
		user, err := gothic.CompleteUserAuth(context.Writer, context.Request)
		if err != nil {
			fmt.Fprintln(context.Writer, err)
			return
		}

		context.JSON(200, user)
	})

	r.GET("/logout/:provider", func(context *gin.Context) {
		gothic.Logout(context.Writer, context.Request)

		context.Header("Location", "/")
		context.Status(http.StatusTemporaryRedirect)
	})

	r.GET("/auth/:provider", func(context *gin.Context) {
		// try to get the user without re-authenticating
		if user, err := gothic.CompleteUserAuth(context.Writer, context.Request); err == nil {
			context.Set("user", user)
			context.Next()
		} else if url, err := gothic.GetAuthURL(context.Writer, context.Request); err == nil {
			context.JSON(200, gin.H{
				"auth_url": url,
			})
		} else {
			context.Error(err)
			context.Status(500)
		}
	})

	session, err := mgo.Dial("mongodb://mongodb:27017")

	if err != nil {
		panic(err)
	}
	defer session.Close()

	r.Use(middleware.Store(session))

	party := r.Group("/party")

	party.POST("/", controllers.CreateParty)
	party.GET("/:join_code", controllers.GetParty)

	// Listen and Server in 0.0.0.0:8080
	r.Run(":8081")
}
