package main

import (
	"github.com/gin-gonic/gin"
	"gopkg.in/mgo.v2"

	"dubclan/api/middleware"
	"dubclan/api/controllers"
	"net/http"
	"fmt"
	//"github.com/zmb3/spotify"
	"github.com/terev/goth/gothic"
	provider_spotify "github.com/terev/goth/providers/spotify"
	"github.com/markbates/goth"
	"os"
	"strings"
	"github.com/dgrijalva/jwt-go"
	"errors"
	"encoding/base64"
)

var signing_key []byte

func init() {
	if key_data, exists := os.LookupEnv("SIGNING_KEY"); exists {
		if key, err := base64.StdEncoding.DecodeString(key_data); err == nil {
			signing_key = key
		} else {
			panic(err)
		}
	} else {
		panic(errors.New("the signing key must be provided in an environment variable"))
	}
}

func main() {
	// Disable Console Color
	// gin.DisableConsoleColor()
	r := gin.Default()

	// Ping test
	r.GET("/ping", func(context *gin.Context) {
		user := context.MustGet("user").(goth.User)
		context.JSON(200, user)
	})

	goth.UseProviders(
		provider_spotify.New(os.Getenv("SPOTIFY_ID"), os.Getenv("SPOTIFY_SECRET"), "http://10.0.2.2:8081/auth/spotify/callback"),
	)

	gothic.GetProviderName = func(req *http.Request) (string, error) {
		parts := strings.Split(req.URL.Path, "/")

		return parts[2], nil
	}

	r.GET("/auth/:provider/callback", func(context *gin.Context) {
		identity, err := gothic.CompleteUserAuth(context.Writer, context.Request)
		if err != nil {
			fmt.Fprintln(context.Writer, err)
			return
		}

		type APIClaims struct {
			jwt.StandardClaims
			AccessToken string `json:"access_token"`
		}

		claims := APIClaims{
			StandardClaims: jwt.StandardClaims{
				ExpiresAt: identity.ExpiresAt.Unix(),
				Issuer:    "qitup.ca",
				Subject:   "id",
			},
		}

		if identity.Provider == "spotify" {
			claims.AccessToken = identity.AccessToken
		}

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

		// Sign and get the complete encoded token as a string using the secret
		token_blob, err := token.SignedString(signing_key)

		if err != nil {
			context.Error(err)
		}

		payload := gin.H{"token": token_blob}

		// Pass the JWT token, and identity access token to the client
		context.JSON(200, payload)
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
		} else {
			gothic.BeginAuthHandler(context.Writer, context.Request)
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
