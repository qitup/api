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
	"strings"
	"github.com/gin-contrib/sessions"
	"gopkg.in/dgrijalva/jwt-go.v3"
	jwt_middleware "github.com/appleboy/gin-jwt"
	"time"
	"github.com/urfave/cli"
	"dubclan/api/models"
	"github.com/olahol/melody"
	"encoding/json"
	"net/url"
	"log"
	"gopkg.in/mgo.v2/bson"
	"dubclan/api/party"
	"encoding/base64"
	"dubclan/api/players"
	"github.com/unrolled/secure"
)

func SecureHeaders(cli *cli.Context) gin.HandlerFunc {
	secureMiddleware := secure.New(secure.Options{
		IsDevelopment: cli.String("mode") == "debug" || cli.String("mode") == "test",

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

func api(cli *cli.Context) error {
	var signing_key []byte

	key_data := cli.String("signing-key")

	// Decode the signing key
	if key, err := base64.StdEncoding.DecodeString(key_data); err == nil {
		signing_key = key
	} else {
		return err
	}

	r := gin.Default()
	r.Use(SecureHeaders(cli))

	m := melody.New()
	m.Config.MaxMessageSize = 8192

	PartySessions := map[string]*party.Session{}

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

	index := mgo.Index{
		Key:    []string{"join_code"},
		Unique: true,
	}
	err = session.DB(cli.String("database")).C(models.PARTY_COLLECTION).EnsureIndex(index)
	if err != nil {
		panic(err)
	}

	r.Use(store.Middleware(session, cli))

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
		provider_spotify.New(
			cli.String("spotify-id"),
			cli.String("spotify-secret"),
			callback_url+"/auth/spotify/callback",
			"streaming", "user-library-read", "user-read-private", "user-read-playback-state", "user-modify-playback-state", "user-read-currently-playing",
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
		token_blob, err := token.SignedString(signing_key)

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
		if _, err := gothic.CompleteUserAuth(context.Writer, context.Request); err == nil {
			context.Next()
		} else {
			gothic.BeginAuthHandler(context.Writer, context.Request)
		}
	})

	party_group := r.Group("/party", auth_middleware.MiddlewareFunc())

	// Party creation route
	party_group.POST("/", func(context *gin.Context) {
		conn := pool.Get()
		defer conn.Close()

		if err := conn.Err(); err != nil {
			context.AbortWithError(500, err)
			return
		}

		controllers.CreateParty(conn, context, cli)
	})

	party_group.GET("/join", func(context *gin.Context) {
		conn := pool.Get()
		defer conn.Close()

		if err := conn.Err(); err != nil {
			context.AbortWithError(500, err)
			return
		}

		controllers.JoinParty(conn, context, cli, PartySessions)
	})

	party_group.GET("/connect/:code", func(context *gin.Context) {
		conn := pool.Get()
		defer conn.Close()

		if err := conn.Err(); err != nil {
			context.AbortWithError(500, err)
			return
		}

		connect_token, _ := url.PathUnescape(context.Param("code"))

		// Handle the request if the url is valid
		if success, party_id, err := party.FinishConnect(conn, connect_token); success && err == nil {
			if err := m.HandleRequestWithKeys(context.Writer, context.Request, gin.H{
				"party_id": party_id,
				"user_id":  context.GetString("userID"),
			}); err != nil {
				context.AbortWithError(500, err)
			}
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

	// Add session to the party map
	m.HandleConnect(func(s *melody.Session) {
		conn := pool.Get()
		defer conn.Close()

		if err := conn.Err(); err != nil {
			return
		}

		party_id, _ := s.Get("party_id")

		// Notify others this attendee has become active
		if session, ok := PartySessions[party_id.(string)]; ok {
			attendee_count := len(session.Sessions)
			log.Println("Connected to party with", attendee_count, "other active attendees")

			res, _ := json.Marshal(gin.H{
				"type": "attendee.active",
				"user": s.MustGet("user_id"),
			})

			for _, sess := range session.Sessions {
				if writeErr := sess.Write(res); writeErr != nil {
					log.Println(writeErr)
				}
			}

			session.Sessions[s] = s
		} else {
			log.Println("Connected to party with 0 other active attendees")

			if queue, err := party.TryResumeQueue(conn, party_id.(string)); err == nil {
				PartySessions[party_id.(string)] = &party.Session{
					Sessions: map[*melody.Session]*melody.Session{s: s},
					Queue:    queue,
				}
			}
		}

		// Say hello to the user when they connect
		msg, _ := json.Marshal(gin.H{
			"type": "hello",
		})

		s.Write([]byte(msg))
	})

	// Cleanup session from the party map
	m.HandleDisconnect(func(s *melody.Session) {
		party_id, _ := s.Get("party_id")

		if session, ok := PartySessions[party_id.(string)]; ok {
			delete(session.Sessions, s)

			attendee_count := len(session.Sessions)
			log.Println("Left session with", attendee_count, "other active attendees")

			if attendee_count == 0 {
				delete(PartySessions, party_id.(string))
			} else {
				// Notify others this attendee has disconnected
				res, _ := json.Marshal(gin.H{
					"type": "attendee.offline",
					"user": s.MustGet("user_id"),
				})

				for _, sess := range session.Sessions {
					if writeErr := sess.Write(res); writeErr != nil {
						log.Println(writeErr)
					}
				}
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

		party_id := s.MustGet("party_id")
		session := PartySessions[party_id.(string)]

		user_id := s.MustGet("user_id").(string)

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
				conn := pool.Get()
				defer conn.Close()

				if err := conn.Err(); err != nil {
					log.Println("Failed opening redis connection", err)
					return
				}

				item, err := party.UnmarshalItem(*msg["item"])

				if err != nil {
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

				item.Added(bson.ObjectIdHex(user_id))

				if err := session.Queue.Push(conn, party_id.(string), item); err == nil {
					event, err := json.Marshal(gin.H{
						"item": item,
						"type": msg_type,
					})

					if err != nil {
						log.Println(err)
						return
					}

					for _, sess := range session.Sessions {
						if writeErr := sess.Write(event); writeErr != nil {
							log.Println(writeErr)
						}
					}
				}
				break
			case "player.event":
				var event player.Event
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

	me := r.Group("/me", auth_middleware.MiddlewareFunc())
	me.GET("/", controllers.Me)

	return r.Run(":" + cli.String("port"))
}
