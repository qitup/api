package main

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"dubclan/api/controllers"
	"dubclan/api/models"
	spotifyPlayer "dubclan/api/player/spotify"
	"dubclan/api/store"

	jwtMiddleware "github.com/appleboy/gin-jwt"
	"github.com/gin-gonic/gin"
	"github.com/markbates/goth"
	"github.com/olahol/melody"
	"github.com/terev/goth/gothic"
	"github.com/unrolled/secure"
	"github.com/urfave/cli"
	"github.com/zmb3/spotify"
	"golang.org/x/oauth2/clientcredentials"
	"gopkg.in/dgrijalva/jwt-go.v3"
	"gopkg.in/mgo.v2"
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
	keyData := cli.String("signing-key")

	// Decode the signing key
	if key, err := base64.StdEncoding.DecodeString(keyData); err == nil {
		return key, nil
	} else {
		return nil, err
	}
}

func api(cli *cli.Context) error {
	signingKey, err := decodeSigningKey(cli)
	if err != nil {
		panic(err)
	}

	router := gin.Default()
	router.Use(secureHeaders(cli))

	m := melody.New()
	m.Config.MaxMessageSize = 8192

	redisStore := store.NewRedisStore(10, "tcp", "redis:6379", "")

	sessionStore, err := redisStore.GetSessionStore([]byte(cli.String("session-secret")))
	if err != nil {
		panic(err)
	}
	gothic.Store = sessionStore

	session, err := mgo.Dial("mongodb://mongodb:27017")

	if err != nil {
		panic(err)
	}
	defer session.Close()

	mongoStore := store.NewMongoStore(session, cli.String("database"))

	index := mgo.Index{
		Key:    []string{"join_code"},
		Unique: true,
	}

	err = session.DB(cli.String("database")).C(models.PARTY_COLLECTION).EnsureIndex(index)
	if err != nil {
		panic(err)
	}

	index = mgo.Index{
		Key:    []string{"email"},
		Unique: true,
	}

	err = session.DB(cli.String("database")).C(models.USER_COLLECTION).EnsureIndex(index)
	if err != nil {
		panic(err)
	}

	// Initialize controllers
	var (
		userController  = controllers.NewUserController(mongoStore, redisStore, signingKey)
		partyController = controllers.NewPartyController(mongoStore, redisStore)
	)

	var httpProtocol string
	if cli.Bool("secured") {
		httpProtocol = "https"
	} else {
		httpProtocol = "http"
	}

	var callbackUrl string
	if cli.Bool("public") {
		callbackUrl = httpProtocol + "://" + cli.String("host")
	} else {
		callbackUrl = httpProtocol + "://" + cli.String("host") + ":" + cli.String("port")
	}

	goth.UseProviders(spotifyPlayer.InitProvider(callbackUrl, cli))

	gothic.GetProviderName = func(req *http.Request) (string, error) {
		parts := strings.Split(req.URL.Path, "/")

		return parts[2], nil
	}

	authMiddleware := &jwtMiddleware.GinJWTMiddleware{
		Realm:      "api",
		Key:        signingKey,
		Timeout:    time.Hour * 5,
		MaxRefresh: time.Hour * 24,

		TimeFunc: time.Now,

		IdentityHandler: func(claims jwt.MapClaims) string {
			return claims["sub"].(string)
		},
	}

	router.POST("/login", func(context *gin.Context) {
		userController.Login(context, cli.String("host"))
	})

	router.POST("/signup", func(context *gin.Context) {
		userController.Signup(context, cli.String("host"))
	})

	router.GET("/auth/:provider/callback", func(context *gin.Context) {
		identity, err := gothic.CompleteUserAuth(context.Writer, context.Request)
		if err != nil {
			context.AbortWithError(500, err)
			return
		}

		user, err := userController.CompleteUserAuth(context, models.Identity(identity))
		if err != nil {
			context.AbortWithError(500, err)
			return
		}

		tokenBlob, err := user.NewToken(cli.String("host"), signingKey)
		if err != nil {
			context.Error(err)
		}

		res := gin.H{
			"token": tokenBlob,
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
		gothic.BeginAuthHandler(context.Writer, context.Request)
	})

	router.GET("/spotify/token", authMiddleware.MiddlewareFunc(), func(context *gin.Context) {
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

	partyGroup := router.Group("/party", authMiddleware.MiddlewareFunc())

	partyGroup.GET("/", partyController.Get)

	// party creation route
	partyGroup.POST("/", func(context *gin.Context) {
		partyController.Create(context, cli)
	})

	partyGroup.GET("/join", func(context *gin.Context) {
		partyController.Join(context, cli)
	})

	partyGroup.GET("/leave", partyController.Leave)

	partyGroup.GET("/connect/:code", func(context *gin.Context) {
		partyController.Connect(context, m)
	})

	partyGroup.POST("/push", partyController.PushHTTP)

	partyGroup.GET("/player/play", partyController.Play)

	partyGroup.GET("/player/pause", partyController.Pause)

	partyGroup.GET("/player/next", partyController.Next)

	// Handle channel connections
	m.HandleConnect(func(s *melody.Session) {
		if channel, ok := s.Get("channel"); ok {
			switch channel {
			case "party":
				partyController.HandleConnect(s)
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
				partyController.HandleDisconnect(s)
				break
			default:
				log.Println("Disconnect from invalid channel detected, closing...")
			}
		}
	})

	m.HandleMessage(func(s *melody.Session, data []byte) {
		var msg map[string]*json.RawMessage

		if err := json.Unmarshal(data, &msg); err != nil {
			errorRes, _ := json.Marshal(gin.H{
				"type": "error",
				"error": gin.H{
					"code": "invalid_json",
					"msg":  "Invalid JSON message",
				},
			})

			s.Write([]byte(errorRes))
			return
		}

		if raw, ok := msg["type"]; ok {
			var msgType string
			if err := json.Unmarshal(*raw, &msgType); err != nil {
				log.Println(err)
				return
			}

			switch msgType {
			case "ping":
				res, _ := json.Marshal(gin.H{
					"type": "pong",
					"time": time.Now().Unix(),
				})

				s.Write([]byte(res))
				break
			case "queue.push":
				partyController.PushSocket(s, *msg["item"])
				break
			}
		} else {
			errorRes, _ := json.Marshal(gin.H{
				"type": "error",
				"error": gin.H{
					"code": "invalid_message",
					"msg":  "Message missing type field",
				},
			})

			s.Write([]byte(errorRes))
		}
	})

	me := router.Group("/me", authMiddleware.MiddlewareFunc())
	me.GET("/", userController.Me)

	return router.Run(":" + cli.String("port"))
}
