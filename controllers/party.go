package controllers

import (
	"dubclan/api/models"
	"dubclan/api/party"
	"github.com/gin-gonic/gin"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2"
	"github.com/urfave/cli"
	"net/url"
	"dubclan/api/store"
	"github.com/olahol/melody"
	"encoding/json"
	"log"
	"github.com/garyburd/redigo/redis"
	"errors"
)

type PartyController struct {
	baseController
	party_sessions map[string]*party.Session
}

func NewPartyController(mongo *store.MongoStore, redis *store.RedisStore) PartyController {
	return PartyController{
		baseController: newBaseController(mongo, redis),
		party_sessions: make(map[string]*party.Session),
	}
}

func (c *PartyController) Get(context *gin.Context) {
	session, db := c.Mongo.DB()
	defer session.Close()

	code := context.Query("code")

	party_record, err := models.PartyByCode(db, code)

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

	context.JSON(200, party_record)
}

func (c *PartyController) Create(context *gin.Context, cli *cli.Context) {
	session, db := c.Mongo.DB()
	defer session.Close()

	var data struct {
		Name     string          `json:"name"`
		JoinCode string          `json:"join_code"`
		Settings models.Settings `json:"settings"`
	}

	user_id := bson.ObjectIdHex(context.MustGet("userID").(string))

	if context.BindJSON(&data) == nil {
		party_record := models.NewParty(user_id, data.Name, data.JoinCode)

		err := party_record.Insert(db)

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
		if err := party_record.WithHost(db); err != nil {
			context.AbortWithError(500, err)
			return
		}

		conn, err := c.Redis.GetConnection()
		if err != nil {
			context.AbortWithError(500, err)
		}
		defer conn.Close()

		queue := party.NewQueue()
		session := party.NewSession(&party_record, queue, c.Mongo, c.Redis)

		c.party_sessions[party_record.ID.Hex()] = session

		connect_token, err := party.InitiateConnect(conn, party_record, bson.ObjectIdHex(context.GetString("userID")))

		var ws_protocol string
		if cli.Bool("secured") {
			ws_protocol = "wss"
		} else {
			ws_protocol = "ws"
		}

		var connect_url string
		if cli.Bool("public") {
			connect_url = ws_protocol + "://" + cli.String("host")
		} else {
			connect_url = ws_protocol + "://" + cli.String("host") + ":" + cli.String("port")
		}

		switch err {
		case nil:
			context.JSON(201, gin.H{
				"url":   connect_url + "/party/connect/" + url.PathEscape(connect_token),
				"party": party_record,
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

	party_record, err := models.PartyByCode(db, code)

	party_session, ok := c.party_sessions[party_record.ID.Hex()]

	conn, err := c.Redis.GetConnection()
	if err != nil {
		context.AbortWithError(500, err)
	}
	defer conn.Close()

	if ok {
		party_record = party_session.GetParty()
	} else if queue, err := party.ResumeQueue(conn, party_record.ID.Hex()); err == nil {
		party_session = party.NewSession(party_record, queue, c.Mongo, c.Redis)

		c.party_sessions[party_record.ID.Hex()] = party_session
	} else if err == redis.ErrNil {
		queue = party.NewQueue()
		party_session = party.NewSession(party_record, queue, c.Mongo, c.Redis)

		c.party_sessions[party_record.ID.Hex()] = party_session
	} else {
		context.AbortWithError(500, err)
	}

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

	user, err := models.UserByID(db, bson.ObjectIdHex(context.GetString("userID")))

	if err != nil {
		context.AbortWithError(500, err)
	}

	attendee := models.NewAttendee(*user)

	connect_token, err := party.InitiateConnect(conn, *party_record, user.ID)

	if attendee.UserId != party_record.HostID {
		if err := party_record.AddAttendee(db, &attendee); err != nil && err != mgo.ErrNotFound {
			context.AbortWithError(500, err)
		}

		if err := party_session.AttendeesChanged(); err != nil {
			context.AbortWithError(500, err)
		}
	}

	var ws_protocol string
	if cli.Bool("secured") {
		ws_protocol = "wss"
	} else {
		ws_protocol = "ws"
	}

	var connect_url string
	if cli.Bool("public") {
		connect_url = ws_protocol + "://" + cli.String("host")
	} else {
		connect_url = ws_protocol + "://" + cli.String("host") + ":" + cli.String("port")
	}

	switch err {
	case nil:
		res := gin.H{
			"url":   connect_url + "/party/connect/" + url.PathEscape(connect_token),
			"party": party_record,
			"queue": party_session.GetQueue(),
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
	party_id := context.Query("id")
	user_id := bson.ObjectIdHex(context.MustGet("userID").(string))

	session, db := c.Mongo.DB()
	defer session.Close()

	party_session, ok := c.party_sessions[party_id]

	var party_record *models.Party
	var err error

	if ok {
		party_record = party_session.GetParty()
	} else {
		party_record, err = models.PartyByID(db, bson.ObjectIdHex(party_id))

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

	if user_id == party_record.HostID {
		// Handle a host leaving

		if len(party_record.Attendees) == 0 {
			// Cleanup the party if it's empty
			if ok {
				party_session.Stop()
				delete(c.party_sessions, party_record.ID.Hex())
			}

			if err := party_record.Remove(db); err != nil {
				log.Println(err)
			}

			context.JSON(200, bson.M{})
			return
		} else if transfer_id, ok := context.GetQuery("transfer_to"); ok {
			transfer_to := bson.ObjectIdHex(transfer_id)

			if err := party_record.TransferHost(db, transfer_to); err != nil {
				context.AbortWithError(500, err)
			}

			if err := party_record.WithHost(db); err != nil {
				context.AbortWithError(500, err)
			}

			if ok {
				transfer_user, err := models.UserByID(db, transfer_to)

				if err != nil {
					context.AbortWithError(500, err)
					return
				}

				party_session.TransferHost(*transfer_user)
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

		err = party_record.RemoveAttendee(db, user_id)

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

		context.JSON(200, gin.H{})
	}
}

func (c *PartyController) Connect(context *gin.Context, m *melody.Melody) {
	connect_token, err := url.PathUnescape(context.Param("code"))
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
	if success, party_id, err := party.FinishConnect(conn, connect_token); success && err == nil {
		if err := m.HandleRequestWithKeys(context.Writer, context.Request, gin.H{
			"channel":  "party",
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
}

func (c *PartyController) HandleConnect(s *melody.Session) {
	party_id := s.MustGet("party_id").(string)

	// Notify others this attendee has become active
	if session, ok := c.party_sessions[party_id]; ok {
		session.ClientConnected(s)
	} else {
		log.Printf("No party session exists for (%s), something's fucky", party_id)
		return
	}

	// Say hello to the user when they connect
	msg, _ := json.Marshal(gin.H{
		"type": "hello",
	})

	s.Write([]byte(msg))
}

func (c *PartyController) HandleDisconnect(s *melody.Session) {
	party_id, _ := s.Get("party_id")

	if session, ok := c.party_sessions[party_id.(string)]; ok {
		// Cleanup session from the party map
		session.ClientDisconnected(s)
	} else {
		log.Printf("No party session exists for (%s), something's fucky", party_id)
	}
}

func (c *PartyController) PushSocket(s *melody.Session, raw_item json.RawMessage) {
	u := &models.ItemUnpacker{}

	err := json.Unmarshal(raw_item, u)

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

	item := u.Result

	user_id := s.MustGet("user_id").(string)
	item.Added(bson.ObjectIdHex(user_id))

	party_id, _ := s.Get("party_id")

	if session, ok := c.party_sessions[party_id.(string)]; ok {
		// Cleanup session from the party map
		if err := session.Push(item); err != nil {
			log.Println("Failed pushing item to queue", err)
			return
		}
	} else {
		log.Printf("No party session exists for (%s), something's fucky", party_id)
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

	user_id := context.MustGet("userID").(string)
	item.Added(bson.ObjectIdHex(user_id))

	party_id := context.Query("id")

	if session, ok := c.party_sessions[party_id]; ok {
		// Cleanup session from the party map
		if err := session.Push(item); err != nil {
			context.AbortWithError(500, err)
		}
	} else {
		context.AbortWithError(500, errors.New("No party session exists for (%s), something's fucky"+party_id))
	}
}

func (c *PartyController) Play(context *gin.Context) {
	party_id := bson.ObjectIdHex(context.Query("id"))

	if !party_id.Valid() {
		context.AbortWithStatusJSON(400, gin.H{
			"type": "error",
			"error": gin.H{
				"code": "invalid_party",
				"msg":  "Invalid party",
			},
		})
	}

	if session, ok := c.party_sessions[party_id.Hex()]; ok {
		if err := session.Play(); err != nil {
			context.AbortWithError(500, err)
		} else {
			context.JSON(200, gin.H{})
		}
	}
}

func (c *PartyController) Pause(context *gin.Context) {
	party_id := bson.ObjectIdHex(context.Query("id"))

	if !party_id.Valid() {
		context.AbortWithStatusJSON(400, gin.H{
			"type": "error",
			"error": gin.H{
				"code": "invalid_party",
				"msg":  "Invalid party",
			},
		})
	}

	if session, ok := c.party_sessions[party_id.Hex()]; ok {
		if err := session.Pause(); err != nil {
			context.AbortWithError(500, err)
		} else {
			context.JSON(200, gin.H{})
		}
	}
}

func (c *PartyController) Next(context *gin.Context) {
	party_id := bson.ObjectIdHex(context.Query("id"))

	if !party_id.Valid() {
		context.AbortWithStatusJSON(400, gin.H{
			"type": "error",
			"error": gin.H{
				"code": "invalid_party",
				"msg":  "Invalid party",
			},
		})
	}

	if session, ok := c.party_sessions[party_id.Hex()]; ok {
		if err := session.Next(); err != nil {
			context.AbortWithError(500, err)
		} else {
			context.JSON(200, gin.H{})
		}
	}
}
