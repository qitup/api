package main

import (
	"github.com/gin-gonic/gin"
	"gopkg.in/mgo.v2"
	"dubclan/api/store"
	"dubclan/api/controllers"
	"net/http"
	spotify_provider "github.com/markbates/goth/providers/spotify"
	"github.com/markbates/goth"
	"github.com/terev/goth/gothic"
	"strings"
	"gopkg.in/dgrijalva/jwt-go.v3"
	jwt_middleware "github.com/appleboy/gin-jwt"
	"time"
	"github.com/urfave/cli"
	"dubclan/api/models"
	"github.com/olahol/melody"
	"encoding/json"
	"log"
	"dubclan/api/party"
	"encoding/base64"
	"github.com/unrolled/secure"
	spotify_player "dubclan/api/party/spotify"
	"golang.org/x/oauth2/clientcredentials"
	"github.com/zmb3/spotify"
)

func secureHeaders(cli *cli.Context) gin.HandlerFunc {
	secureMiddleware := secure.New(secure.Options{
		IsDevelopment: cli.String("mode") == "debug" || cli.String("mode") == "test",

		STSSeconds:            315360000,
		STSIncludeSubdomains:  true,
		STSPreload:            true,
		FrameDeny:             true,
		ContentTypeNosniff:    true,
		ContentSecurityPolicy: "default-src 'self'",
	})

	return func() gin.HandlerFunc {
		return func(c *gin.Context) {
			err := secureMiddleware.Process(c.Writer, c.Request)

			// If there was an error, do not continue.
			if err != nil {
				c.Abort()
				return
			}

			// Avoid header rewrite if response is a redirection.
			if status := c.Writer.Status(); status > 300 && status < 399 {
				c.Abort()
			}
		}
	}()
}

func decodeSigningKey(cli *cli.Context) ([]byte, error) {
	key_data := cli.String("signing-key")

	// Decode the signing key
	if key, err := base64.StdEncoding.DecodeString(key_data); err == nil {
		return key, nil
	} else {
		return nil, err
	}
}

func api(cli *cli.Context) error {
	signing_key, err := decodeSigningKey(cli)
	if err != nil {
		panic(err)
	}

	router := gin.Default()
	router.Use(secureHeaders(cli))

	m := melody.New()
	m.Config.MaxMessageSize = 8192

	redis_store := store.NewRedisStore(10, "tcp", "redis:6379", "")

	session_store, err := redis_store.GetSessionStore([]byte(cli.String("session-secret")))
	if err != nil {
		panic(err)
	}
	gothic.Store = session_store

	session, err := mgo.Dial("mongodb://mongodb:27017")

	if err != nil {
		panic(err)
	}
	defer session.Close()

	mongo_store := store.NewMongoStore(session, cli.String("database"))

	index := mgo.Index{
		Key:    []string{"join_code"},
		Unique: true,
	}

	err = session.DB(cli.String("database")).C(models.PARTY_COLLECTION).EnsureIndex(index)
	if err != nil {
		panic(err)
	}

	// Initialize controllers
	var (
		user_controller  = controllers.NewUserController(mongo_store, redis_store)
		party_controller = controllers.NewPartyController(mongo_store, redis_store)
	)

	var http_protocol string
	if cli.Bool("secured") {
		http_protocol = "https"
	} else {
		http_protocol = "http"
	}

	var callback_url string
	if cli.Bool("public") {
		callback_url = http_protocol + "://" + cli.String("host")
	} else {
		callback_url = http_protocol + "://" + cli.String("host") + ":" + cli.String("port")
	}

	goth.UseProviders(
		spotify_provider.New(
			cli.String("spotify-id"),
			cli.String("spotify-secret"),
			callback_url+"/auth/spotify/callback",
			spotify_player.HostScopes...,
		),
	)

	gothic.GetProviderName = func(req *http.Request) (string, error) {
		parts := strings.Split(req.URL.Path, "/")

		return parts[2], nil
	}

	auth_middleware := &jwt_middleware.GinJWTMiddleware{
		Realm:      "api",
		Key:        signing_key,
		Timeout:    time.Hour,
		MaxRefresh: time.Hour,

		TimeFunc: time.Now,

		IdentityHandler: func(claims jwt.MapClaims) string {
			return claims["sub"].(string)
		},
	}

	router.GET("/auth/:provider/callback", func(context *gin.Context) {
		identity, err := gothic.CompleteUserAuth(context.Writer, context.Request)
		if err != nil {
			context.AbortWithError(500, err)
			return
		}

		user, err := user_controller.CompleteUserAuth(context, models.Identity(identity))
		if err != nil {
			context.AbortWithError(500, err)
			return
		}

		token_blob, err := user.NewToken(cli.String("host"), signing_key)
		if err != nil {
			context.Error(err)
		}

		res := gin.H{
			"token": token_blob,
		}

		if identity.Provider == "spotify" {
			res["access_token"] = identity.AccessToken
		}

		// Pass the JWT token, and identity access token to the client
		context.JSON(200, res)
	})

	router.GET("/logout/:provider", func(context *gin.Context) {
		gothic.Logout(context.Writer, context.Request)

		context.Header("Location", "/")
		context.Status(http.StatusTemporaryRedirect)
	})

	router.GET("/auth/:provider", func(context *gin.Context) {
		// try to get the user without re-authenticating
		if _, err := gothic.CompleteUserAuth(context.Writer, context.Request); err == nil {
			context.Next()
		} else {
			gothic.BeginAuthHandler(context.Writer, context.Request)
		}
	})

	router.GET("/spotify/token", auth_middleware.MiddlewareFunc(), func(context *gin.Context) {
		config := &clientcredentials.Config{
			ClientID:     cli.String("spotify-id"),
			ClientSecret: cli.String("spotify-secret"),
			TokenURL:     spotify.TokenURL,
		}

		token, err := config.Token(context)
		if err != nil {
			context.AbortWithError(500, err)
			return
		}

		context.JSON(200, token)
	})

	party_group := router.Group("/party", auth_middleware.MiddlewareFunc())

	party_group.GET("/", party_controller.Get)

	// party creation route
	party_group.POST("/", func(context *gin.Context) {
		party_controller.Create(context, cli)
	})

	party_group.GET("/join", func(context *gin.Context) {
		party_controller.Join(context, cli)
	})

	party_group.GET("/connect/:code", func(context *gin.Context) {
		party_controller.Connect(context, m)
	})

	party_group.GET("/push", party_controller.PushHTTP)

	party_group.GET("/player/play", party_controller.Play)

	party_group.GET("/player/pause", party_controller.Pause)


	// Handle channel connections
	m.HandleConnect(func(s *melody.Session) {
		if channel, ok := s.Get("channel"); ok {
			switch channel {
			case "party":
				party_controller.HandleConnect(s)
				break
			default:
				log.Println("Connection to invalid channel detected, closing...")
				s.Close()
			}
		}
	})

	// Handle channel disconnections
	m.HandleDisconnect(func(s *melody.Session) {
		if channel, ok := s.Get("channel"); ok {
			switch channel {
			case "party":
				party_controller.HandleDisconnect(s)
				break
			default:
				log.Println("Disconnect from invalid channel detected, closing...")
				s.Close()
			}
		}
	})

	m.HandleMessage(func(s *melody.Session, data []byte) {
		var msg map[string]*json.RawMessage

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

		if raw, ok := msg["type"]; ok {
			var msg_type string
			if err := json.Unmarshal(*raw, &msg_type); err != nil {
				log.Println(err)
				return
			}

			switch msg_type {
			case "ping":
				res, _ := json.Marshal(gin.H{
					"type": "pong",
					"time": time.Now().Unix(),
				})

				s.Write([]byte(res))
				break
			case "queue.push":
				party_controller.PushSocket(s, *msg["item"])
				break
			case "player.event":
				var event party.Event
				err := json.Unmarshal(*msg["event"], &event)

				if err == nil {
					log.Println(event)
				} else {
					log.Println(err)
				}
				break
			}
		} else {
			error_res, _ := json.Marshal(gin.H{
				"type": "error",
				"error": gin.H{
					"code": "invalid_message",
					"msg":  "Message missing type field",
				},
			})

			s.Write([]byte(error_res))
		}
	})

	me := router.Group("/me", auth_middleware.MiddlewareFunc())
	me.GET("/", user_controller.Me)

	return router.Run(":" + cli.String("port"))
}
