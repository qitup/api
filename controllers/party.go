package controllers

import (
	"encoding/json"
	"errors"
	"log"
	"net/url"

	"dubclan/api/models"
	"dubclan/api/party"
	"dubclan/api/store"

	"github.com/garyburd/redigo/redis"
	"github.com/gin-gonic/gin"
	"github.com/olahol/melody"
	"github.com/urfave/cli"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type PartyController struct {
	baseController
	partySessions map[string]*party.Session
}

func NewPartyController(mongo *store.MongoStore, redis *store.RedisStore) PartyController {
	return PartyController{
		baseController: newBaseController(mongo, redis),
		partySessions:  make(map[string]*party.Session),
	}
}

func (c *PartyController) OnSessionClose(id string) {
	if _, ok := c.partySessions[id]; ok {
		delete(c.partySessions, id)
	}
}

func (c *PartyController) Get(context *gin.Context) {
	session, db := c.Mongo.DB()
	defer session.Close()

	code := context.Query("code")

	partyRecord, err := models.PartyByCode(db, code)

	if err == mgo.ErrNotFound {
		context.JSON(400, gin.H{
			"error": gin.H{
				"code": "party_not_found",
				"msg":  "party not found",
			},
		})
		return
	} else if err != nil {
		context.AbortWithError(500, err)
		return
	}

	context.JSON(200, partyRecord)
}

func (c *PartyController) Create(context *gin.Context, cli *cli.Context) {
	session, db := c.Mongo.DB()
	defer session.Close()

	var data struct {
		Name     string          `json:"name"`
		JoinCode string          `json:"join_code"`
		Settings models.Settings `json:"settings"`
	}

	userId := bson.ObjectIdHex(context.MustGet("userID").(string))

	if context.BindJSON(&data) == nil {
		partyRecord := models.NewParty(userId, data.Name, data.JoinCode, data.Settings)

		err := partyRecord.Insert(db)

		if mgo.IsDup(err) {
			context.JSON(400, gin.H{
				"code": "duplicate_party",
				"msg":  "There is an active party with the same join code",
			})
			return
		} else if err != nil {
			context.AbortWithError(500, err)
			return
		}

		// Get the host details
		if err := partyRecord.WithHost(db); err != nil {
			context.AbortWithError(500, err)
			return
		}

		conn, err := c.Redis.GetConnection()
		if err != nil {
			context.AbortWithError(500, err)
		}
		defer conn.Close()

		queue := party.NewQueue()
		session := party.NewSession(&partyRecord, queue, c.Mongo, c.Redis, c.OnSessionClose)

		c.partySessions[partyRecord.ID.Hex()] = session

		connectToken, err := party.InitiateConnect(conn, partyRecord, bson.ObjectIdHex(context.GetString("userID")))

		var wsProtocol string
		if cli.Bool("secured") {
			wsProtocol = "wss"
		} else {
			wsProtocol = "ws"
		}

		var connectUrl string
		if cli.Bool("public") {
			connectUrl = wsProtocol + "://" + cli.String("host")
		} else {
			connectUrl = wsProtocol + "://" + cli.String("host") + ":" + cli.String("port")
		}

		switch err {
		case nil:
			context.JSON(201, gin.H{
				"url":   connectUrl + "/party/connect/" + url.PathEscape(connectToken),
				"party": partyRecord,
				"queue": queue,
			})
			return
		case party.ConnectTokenIssued:
			context.JSON(403, gin.H{
				"error": gin.H{
					"code": "already_issued",
					"msg":  "Connect token already issued",
				}})
			return
		default:
			context.AbortWithError(500, err)
			return
		}
	} else {
		context.Status(400)
	}
}

// Create unique join url for user
//		join_token: sha2(user_id + join_code)
// set key in redis with 30 sec ttl
// 		SETEX join_token 30
func (c *PartyController) Join(context *gin.Context, cli *cli.Context) {
	session, db := c.Mongo.DB()
	defer session.Close()

	code := context.Query("code")

	partyRecord, err := models.PartyByCode(db, code)

	if err == mgo.ErrNotFound {
		context.JSON(400, gin.H{
			"error": gin.H{
				"code": "party_not_found",
				"msg":  "party not found",
			},
		})
		return
	} else if err != nil {
		context.AbortWithError(500, err)
		return
	}

	partySession, ok := c.partySessions[partyRecord.ID.Hex()]

	conn, err := c.Redis.GetConnection()
	if err != nil {
		context.AbortWithError(500, err)
	}
	defer conn.Close()

	if ok {
		partyRecord = partySession.GetParty()
	} else if queue, err := party.ResumeQueue(conn, partyRecord.ID.Hex()); err == nil {
		partySession = party.NewSession(partyRecord, queue, c.Mongo, c.Redis, c.OnSessionClose)

		c.partySessions[partyRecord.ID.Hex()] = partySession
	} else if err == redis.ErrNil {
		queue = party.NewQueue()
		partySession = party.NewSession(partyRecord, queue, c.Mongo, c.Redis, c.OnSessionClose)

		c.partySessions[partyRecord.ID.Hex()] = partySession
	} else {
		context.AbortWithError(500, err)
	}

	user, err := models.UserByID(db, bson.ObjectIdHex(context.GetString("userID")))

	if err != nil {
		context.AbortWithError(500, err)
		return
	}

	connectToken, err := party.InitiateConnect(conn, *partyRecord, user.ID)

	if user.ID != partyRecord.HostID {
		attendee := models.NewAttendee(*user)

		if err := partyRecord.AddAttendee(db, &attendee); err != nil && err != mgo.ErrNotFound {
			context.AbortWithError(500, err)
			return
		}

		if err := partySession.AttendeesChanged(); err != nil {
			context.AbortWithError(500, err)
			return
		}
	}

	var wsProtocol string
	if cli.Bool("secured") {
		wsProtocol = "wss"
	} else {
		wsProtocol = "ws"
	}

	var connectUrl string
	if cli.Bool("public") {
		connectUrl = wsProtocol + "://" + cli.String("host")
	} else {
		connectUrl = wsProtocol + "://" + cli.String("host") + ":" + cli.String("port")
	}

	switch err {
	case nil:
		res := gin.H{
			"url":   connectUrl + "/party/connect/" + url.PathEscape(connectToken),
			"party": partyRecord,
			"queue": partySession.GetQueue(),
		}

		context.JSON(200, res)
		break
	case party.ConnectTokenIssued:
		context.JSON(403, gin.H{
			"error": gin.H{
				"code": "already_issued",
				"msg":  "Connect token already issued",
			}})
		break
	default:
		context.AbortWithError(500, err)
	}
}

func (c *PartyController) Leave(context *gin.Context) {
	partyId := context.Query("id")
	userId := bson.ObjectIdHex(context.MustGet("userID").(string))

	session, db := c.Mongo.DB()
	defer session.Close()

	partySession, sessionExists := c.partySessions[partyId]

	var partyRecord *models.Party
	var err error

	if sessionExists {
		partyRecord = partySession.GetParty()
	} else {
		partyRecord, err = models.PartyByID(db, bson.ObjectIdHex(partyId))

		if err == mgo.ErrNotFound {
			context.JSON(400, gin.H{
				"error": gin.H{
					"code": "party_not_found",
					"msg":  "party not found",
				},
			})
			return
		} else if err != nil {
			context.AbortWithError(500, err)
			return
		}
	}

	if userId == partyRecord.HostID {
		// Handle a host leaving

		if len(partyRecord.Attendees) == 0 {
			// Cleanup the party if it's empty

			if sessionExists {
				partySession.Close()
			} else if err := partyRecord.Remove(db); err != nil {
				context.AbortWithError(500, err)
				return
			}

			context.JSON(200, bson.M{})
			return
		} else if _, ok := context.GetQuery("end_party"); ok {
			// Cleanup the party and notify the guests of close

			if sessionExists {
				partySession.Close()
			} else if err := partyRecord.Remove(db); err != nil {
				context.AbortWithError(500, err)
				return
			}

			context.JSON(200, bson.M{})
			return
		} else if transferId, ok := context.GetQuery("transfer_to"); ok {
			transferTo := bson.ObjectIdHex(transferId)

			if err := partyRecord.TransferHost(db, transferTo); err != nil {
				context.AbortWithError(500, err)
				return
			}

			if err := partyRecord.WithHost(db); err != nil {
				context.AbortWithError(500, err)
			}

			if sessionExists {
				if err := partySession.AttendeesChanged(); err != nil {
					context.AbortWithError(500, err)
				}

				transferUser, err := models.UserByID(db, transferTo)

				if err != nil {
					context.AbortWithError(500, err)
					return
				}

				partySession.TransferHost(*transferUser)
			}

			context.JSON(200, gin.H{})
			return

		} else {
			// Didn't specify someone to transfer to, deny req
			context.JSON(400, bson.M{
				"error": gin.H{
					"code": "transfer_unspecified",
					"msg":  "user to transfer to not specified",
				},
			})
			return
		}
	} else {
		// Handle an attendee leaving

		err = partyRecord.RemoveAttendee(db, userId)

		if err == mgo.ErrNotFound {
			context.JSON(400, gin.H{
				"error": gin.H{
					"code": "attendee_not_exist",
					"msg":  "not in attendee list",
				},
			})
			return
		} else if err != nil {
			context.AbortWithError(500, err)
			return
		}

		if sessionExists {
			if err := partySession.AttendeesChanged(); err != nil {
				context.AbortWithError(500, err)
			}
		}

		context.JSON(200, gin.H{})
	}
}

func (c *PartyController) Connect(context *gin.Context, m *melody.Melody) {
	connectToken, err := url.PathUnescape(context.Param("code"))
	if err != nil {
		context.AbortWithError(500, err)
		return
	}

	conn, err := c.Redis.GetConnection()
	if err != nil {
		context.AbortWithError(500, err)
		return
	}
	defer conn.Close()

	// Handle the request if the url is valid
	if success, partyId, err := party.FinishConnect(conn, connectToken); success && err == nil {
		if err := m.HandleRequestWithKeys(context.Writer, context.Request, gin.H{
			"channel":  "party",
			"party_id": partyId,
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
}

func (c *PartyController) HandleConnect(s *melody.Session) {
	partyId := s.MustGet("party_id").(string)

	// Notify others this attendee has become active
	if session, ok := c.partySessions[partyId]; ok {
		session.ClientConnected(s)
	} else {
		log.Printf("No party session exists for (%s), something's fucky", partyId)
		return
	}

	// Say hello to the user when they connect
	msg, _ := json.Marshal(gin.H{
		"type": "hello",
	})

	s.Write([]byte(msg))
}

func (c *PartyController) HandleDisconnect(s *melody.Session) {
	partyId, _ := s.Get("party_id")

	if session, ok := c.partySessions[partyId.(string)]; ok {
		// Cleanup session from the party map
		session.ClientDisconnected(s)
	} else {
		log.Printf("No party session exists for (%s), something's fucky", partyId)
	}
}

func (c *PartyController) PushSocket(s *melody.Session, rawItem json.RawMessage) {
	u := &models.ItemUnpacker{}

	err := json.Unmarshal(rawItem, u)

	if err != nil {
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

	item := u.Result

	userId := s.MustGet("user_id").(string)
	item.Added(bson.ObjectIdHex(userId))

	partyId, _ := s.Get("party_id")

	if session, ok := c.partySessions[partyId.(string)]; ok {
		// Cleanup session from the party map
		if err := session.Push(item); err != nil {
			log.Println("Failed pushing item to queue", err)
			return
		}
	} else {
		log.Printf("No party session exists for (%s), something's fucky", partyId)
	}
}

func (c *PartyController) PushHTTP(context *gin.Context) {
	u := &models.ItemUnpacker{}

	err := context.BindJSON(u)

	if err != nil {
		context.JSON(400, gin.H{
			"type": "error",
			"error": gin.H{
				"code": "invalid_json",
				"msg":  "Invalid JSON message",
			},
		})
		return
	}

	item := u.Result

	userId := context.MustGet("userID").(string)
	item.Added(bson.ObjectIdHex(userId))

	partyId := context.Query("id")

	if session, ok := c.partySessions[partyId]; ok {
		// Cleanup session from the party map
		if err := session.Push(item); err != nil {
			context.AbortWithError(500, err)
		}
		context.JSON(200, gin.H{})
	} else {
		context.AbortWithError(500, errors.New("No party session exists for (%s), something's fucky"+partyId))
	}
}

func (c *PartyController) Play(context *gin.Context) {
	partyId := bson.ObjectIdHex(context.Query("id"))

	if !partyId.Valid() {
		context.AbortWithStatusJSON(400, gin.H{
			"type": "error",
			"error": gin.H{
				"code": "invalid_party",
				"msg":  "Invalid party",
			},
		})
	}

	if session, ok := c.partySessions[partyId.Hex()]; ok {
		if err := session.Play(); err != nil {
			context.AbortWithError(500, err)
		} else {
			context.JSON(200, gin.H{})
		}
	}
}

func (c *PartyController) Pause(context *gin.Context) {
	partyId := bson.ObjectIdHex(context.Query("id"))

	if !partyId.Valid() {
		context.AbortWithStatusJSON(400, gin.H{
			"type": "error",
			"error": gin.H{
				"code": "invalid_party",
				"msg":  "Invalid party",
			},
		})
	}

	if session, ok := c.partySessions[partyId.Hex()]; ok {
		if err := session.Pause(); err != nil {
			context.AbortWithError(500, err)
		} else {
			context.JSON(200, gin.H{})
		}
	}
}

func (c *PartyController) Next(context *gin.Context) {
	partyId := bson.ObjectIdHex(context.Query("id"))

	if !partyId.Valid() {
		context.AbortWithStatusJSON(400, gin.H{
			"type": "error",
			"error": gin.H{
				"code": "invalid_party",
				"msg":  "Invalid party",
			},
		})
	}

	if session, ok := c.partySessions[partyId.Hex()]; ok {
		if err := session.Next(); err != nil {
			context.AbortWithError(500, err)
		} else {
			context.JSON(200, gin.H{})
		}
	}
}
