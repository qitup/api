package main

import (
	"github.com/gin-gonic/gin"
	"gopkg.in/mgo.v2"
	"dubclan/api/store"
	"dubclan/api/controllers"
	"net/http"
	provider_spotify "github.com/markbates/goth/providers/spotify"
	"github.com/markbates/goth"
	"github.com/terev/goth/gothic"
	"os"
	"strings"
	"encoding/base64"
	"github.com/gin-contrib/sessions"
	"gopkg.in/dgrijalva/jwt-go.v3"
	jwt_middleware "github.com/appleboy/gin-jwt"
	"time"
	"github.com/urfave/cli"
	"dubclan/api/models"
	"github.com/olahol/melody"
	"encoding/json"
	"net/url"
)

var flags = []cli.Flag{
	cli.StringFlag{
		EnvVar: "PUBLIC_HTTP_HOST",
		Name:   "public-http-host",
	},
	cli.StringFlag{
		EnvVar: "PUBLIC_WS_HOST",
		Name:   "public-ws-host",
	},
	cli.StringFlag{
		EnvVar: "SESSION_SECRET",
		Name:   "session-secret",
		Value:  "secret",
	},
	cli.StringFlag{
		EnvVar: "SIGNING_KEY",
		Name:   "signing-key",
		Usage:  "signing key",
	},
	cli.StringFlag{
		EnvVar: "DATABASE",
		Name:   "database",
		Value:  "dev",
	},
}

func before(context *cli.Context) error {
	key_data := context.String("signing-key")

	// Decode the signing key
	if key, err := base64.StdEncoding.DecodeString(key_data); err == nil {
		context.Set("signing-key", string(key))
	} else {
		return err
	}

	return nil
}

func api(cli *cli.Context) error {
	r := gin.Default()
	m := melody.New()

	pool := store.GetRedisPool(10, "tcp", "redis:6379", "")
	redis_store, err := sessions.NewRedisStoreWithPool(pool, []byte(cli.String("session-secret")))
	if err != nil {
		panic(err)
	}
	gothic.Store = redis_store

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
			cli.String("public-http-host")+"/auth/spotify/callback",
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
			context.AbortWithError(500, err)
			return
		}

		user, err := controllers.CompleteUserAuth(context, models.Identity(identity))

		if err != nil {
			context.AbortWithError(500, err)
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
				Subject:   user.ID.Hex(),
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

	party.GET("/connect/:code", func(context *gin.Context) {
		conn := pool.Get()
		defer conn.Close()

		if err := conn.Err(); err != nil {
			context.AbortWithError(500, err)
			return
		}

		connect_token, _ := url.PathUnescape(context.Param("code"))

		// Delete the connect token to ensure single use
		reply, err := conn.Do("DEL", "jc:"+connect_token)

		if err != nil {
			context.AbortWithError(500, err)
			return
		}

		// Handle the request if the url is valid
		if reply.(int64) == 1 {
			m.HandleRequest(context.Writer, context.Request)
		} else {
			context.JSON(410, gin.H{
				"type": "error",
				"error": gin.H{
					"code": "url_expired",
					"msg":  "Socket URL has expired",
				},
			})
		}
	})

	m.HandleConnect(func(s *melody.Session) {
		msg, _ := json.Marshal(gin.H{
			"type": "hello",
		})

		s.Write([]byte(msg))
	})

	m.HandleMessage(func(s *melody.Session, data []byte) {
		var msg map[string]interface{}

		if err := json.Unmarshal(data, &msg); err != nil {
			error_res, _ := json.Marshal(gin.H{
				"type": "error",
				"error": gin.H{
					"code": "invalid_json",
					"msg":  "Invalid JSON message",
				},
			})

			s.Write([]byte(error_res))
			return
		}

		if msg_type, ok := msg["type"]; ok {
			switch msg_type {
			case "ping":
				res, _ := json.Marshal(gin.H{
					"type": "pong",
					"time": time.Now().Unix(),
				})

				s.Write([]byte(res))
				break
			}
		}
	})

	party.POST("/", func(context *gin.Context) {
		conn := pool.Get()
		defer conn.Close()

		if err := conn.Err(); err != nil {
			context.AbortWithError(500, err)
			return
		}

		controllers.CreateParty(conn, context, cli)
	})

	//party.GET("/:join_code", controllers.GetParty)
	party.GET("/join", func(context *gin.Context) {
		conn := pool.Get()
		defer conn.Close()

		if err := conn.Err(); err != nil {
			context.AbortWithError(500, err)
			return
		}

		controllers.JoinParty(conn, context, cli)
	})

	me := r.Group("/me")

	me.GET("/", controllers.Me)

	// Listen and Server in 0.0.0.0:8080
	return r.Run(":8081")
}
