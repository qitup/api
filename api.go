package main

import (
	"github.com/gin-gonic/gin"
	"gopkg.in/mgo.v2"
	"dubclan/api/store"
	"dubclan/api/controllers"
	"net/http"
	"github.com/terev/goth/gothic"
	provider_spotify "github.com/terev/goth/providers/spotify"
	"github.com/markbates/goth"
	"os"
	"strings"
	"encoding/base64"
	"github.com/gin-contrib/sessions"
	"gopkg.in/dgrijalva/jwt-go.v3"
	jwt_middleware "github.com/appleboy/gin-jwt"
	"time"
	"github.com/urfave/cli"
)

var flags = []cli.Flag{
	cli.StringFlag{
		EnvVar: "SIGNING_KEY",
		Name:   "signing-key",
		Usage:  "signing key",
	},
	cli.StringFlag{
		EnvVar: "DATABASE",
		Name: "database",
		Value: "dev",
	},
}

func before(context *cli.Context) error {
	key_data := context.String("signing-key")

	if key, err := base64.StdEncoding.DecodeString(key_data); err == nil {
		context.Set("signing-key", string(key))
	} else {
		return err
	}

	return nil
}

func api(cli *cli.Context) error {
	// Disable Console Color
	// gin.DisableConsoleColor()
	r := gin.Default()

	redis, err := sessions.NewRedisStore(10, "tcp", "redis:6379", "", []byte("temp_secret"))
	if err != nil {
		panic(err)
	}
	gothic.Store = redis

	session, err := mgo.Dial("mongodb://mongodb:27017")

	if err != nil {
		panic(err)
	}
	defer session.Close()

	r.Use(store.Middleware(session, cli))

	goth.UseProviders(
		provider_spotify.New(
			os.Getenv("SPOTIFY_ID"),
			os.Getenv("SPOTIFY_SECRET"),
			os.Getenv("BASE_HOST")+"/auth/spotify/callback",
			"streaming", "user-library-read",
		),
	)

	gothic.GetProviderName = func(req *http.Request) (string, error) {
		parts := strings.Split(req.URL.Path, "/")

		return parts[2], nil
	}

	auth_middleware := &jwt_middleware.GinJWTMiddleware{
		Realm:      "api",
		Key:        []byte(cli.String("signing-key")),
		Timeout:    time.Hour,
		MaxRefresh: time.Hour,

		TimeFunc: time.Now,

		IdentityHandler: func(claims jwt.MapClaims) string {
			return claims["sub"].(string)
		},
	}

	r.GET("/auth/:provider/callback", func(context *gin.Context) {
		identity, err := gothic.CompleteUserAuth(context.Writer, context.Request)
		if err != nil {
			context.Error(err)
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
		token_blob, err := token.SignedString([]byte(cli.String("signing-key")))

		if err != nil {
			context.Error(err)
		}

		// Pass the JWT token, and identity access token to the client
		context.JSON(200, gin.H{
			"token": token_blob,
		})
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

	r.Use(auth_middleware.MiddlewareFunc())

	party := r.Group("/party")

	party.POST("/", controllers.CreateParty)
	party.GET("/:join_code", controllers.GetParty)

	// Listen and Server in 0.0.0.0:8080
	return r.Run(":8081")
}