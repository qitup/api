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
	"github.com/VividCortex/multitick"
	"time"
)

var ticker = multitick.NewTicker(5*time.Second, 0)

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
		Name     string `json:"name"`
		JoinCode string `json:"join_code"`
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

		attendee := models.NewAttendee(bson.ObjectIdHex(context.GetString("userID")))

		conn, err := c.Redis.GetConnection()
		if err != nil {
			context.AbortWithError(500, err)
		}
		defer conn.Close()

		connect_token, err := party.InitiateConnect(conn, party_record, attendee)

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
				"queue": party.NewQueue(),
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

	attendee := models.NewAttendee(bson.ObjectIdHex(context.GetString("userID")))

	conn, err := c.Redis.GetConnection()
	if err != nil {
		context.AbortWithError(500, err)
	}
	defer conn.Close()

	connect_token, err := party.InitiateConnect(conn, *party_record, attendee)

	if attendee.UserId != party_record.HostID {
		if err := party_record.AddAttendee(db, &attendee); err != nil && err != mgo.ErrNotFound {
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
		}

		// Add the queue's contents to the response if available
		if session, ok := c.party_sessions[party_record.ID.Hex()]; ok {
			res["queue"] = session.Queue
		} else if queue, err := party.ResumeQueue(conn, party_record.ID.Hex()); err == nil {
			res["queue"] = queue
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
	conn, err := c.Redis.GetConnection()
	if err != nil {
		log.Println("Failed obtaining redis connection", err)
		return
	}
	defer conn.Close()

	party_id, _ := s.Get("party_id")

	// Notify others this attendee has become active
	if session, ok := c.party_sessions[party_id.(string)]; ok {
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

		if queue, err := party.ResumeQueue(conn, party_id.(string)); err == nil {
			session := party.NewSession(queue, ticker)
			session.Sessions[s] = s

			c.party_sessions[party_id.(string)] = session
		}
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
		delete(session.Sessions, s)

		attendee_count := len(session.Sessions)
		log.Println("Left session with", attendee_count, "other active attendees")

		if attendee_count == 0 {
			session.Stop()
			delete(c.party_sessions, party_id.(string))
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
	} else {
		log.Printf("No party session exists for (%s), something's fucky", party_id)
	}
}

func (c *PartyController) PushItem(s *melody.Session, raw_item json.RawMessage) {
	item, err := party.UnmarshalItem(raw_item)

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

	user_id := s.MustGet("user_id").(string)
	item.Added(bson.ObjectIdHex(user_id))

	party_id := s.MustGet("party_id")
	session := c.party_sessions[party_id.(string)]

	conn, err := c.Redis.GetConnection()
	if err != nil {
		log.Println("Failed obtaining redis connection", err)
		return
	}
	defer conn.Close()

	if err := session.Queue.Push(conn, party_id.(string), item); err == nil {
		event, err := json.Marshal(gin.H{
			"item": item,
			"type": "queue.push",
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
}
