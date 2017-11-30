package party

import (
	"github.com/olahol/melody"
	"github.com/garyburd/redigo/redis"
	"crypto/sha1"
	"dubclan/api/models"
	"errors"
	"encoding/base64"
	"dubclan/api/party/spotify"
	"dubclan/api/store"
	"github.com/olebedev/emitter"
	"github.com/gin-gonic/gin"
	"log"
	"encoding/json"
	"gopkg.in/mgo.v2/bson"
	"time"
)

const (
	QUEUE_PREFIX     = "queue:"
	JOIN_CODE_PREFIX = "join_code:"
)

var (
	EmptyQueue = errors.New("empty queue")

	Interrupted = errors.New("playback interrupted")

	ConnectTokenIssued = errors.New("connect token is issued for this user")
)

const (
	READY       = iota
	PLAYING
	PAUSED
	INTERRUPTED
)

type Session struct {
	mongo         *store.MongoStore
	redis         *store.RedisStore
	party         *models.Party
	clients       map[string]*melody.Session
	queue         *Queue
	players       map[string]Player
	CurrentPlayer Player
	emitter       *emitter.Emitter
	timeout       <-chan time.Time
	state         int
}

func NewSession(party *models.Party, queue *Queue, mongo *store.MongoStore, redis_store *store.RedisStore) (*Session) {
	session := &Session{
		mongo:   mongo,
		redis:   redis_store,
		party:   party,
		clients: make(map[string]*melody.Session),
		queue:   queue,
		players: make(map[string]Player),
		emitter: emitter.New(10),
		state:   READY,
	}

	go (func() {
		change := session.emitter.On("player.track_finished")
		play := session.emitter.On("player.play")
		interrupt := session.emitter.On("player.interrupted")
		pause := session.emitter.On("player.pause")
		for {
			select {
			case <-change:
				log.Println("CHANGE")
				conn, err := redis_store.GetConnection()

				if err != nil {
					panic(err)
				}

				_, err = session.queue.Pop(conn, session.party.ID.Hex())
				conn.Close()

				if session.CurrentPlayer != nil && !session.CurrentPlayer.HasItems() {
					session.Play()
				}

				event, err := json.Marshal(gin.H{
					"queue": session.queue,
					"type":  "queue.change",
				})

				for _, client := range session.clients {
					if writeErr := client.Write(event); writeErr != nil {
						log.Println(writeErr)
					}
				}
				break
			case <-interrupt:
				session.state = INTERRUPTED
				log.Println("INTERRUPT")
				event, _ := json.Marshal(map[string]interface{}{
					"type": "player.interrupted",
				})

				for _, sess := range session.clients {
					if writeErr := sess.Write(event); writeErr != nil {
						log.Println(writeErr)
					}
				}

				break

			case <-play:
				session.queue.Items[0].Play()
				session.state = PLAYING
				log.Println("PLAY")

				conn, err := redis_store.GetConnection()

				if err != nil {
					panic(err)
				}
				session.queue.UpdateHead(conn, session.party.ID.Hex())
				conn.Close()

				event, _ := json.Marshal(map[string]interface{}{
					"type": "player.play",
				})

				for _, sess := range session.clients {
					if writeErr := sess.Write(event); writeErr != nil {
						log.Println(writeErr)
					}
				}
				break
			case <-pause:
				session.queue.Items[0].Pause()
				session.state = PAUSED
				log.Println("PAUSED")

				conn, err := redis_store.GetConnection()

				if err != nil {
					panic(err)
				}
				session.queue.UpdateHead(conn, session.party.ID.Hex())
				conn.Close()

				event, _ := json.Marshal(map[string]interface{}{
					"type": "player.pause",
				})

				for _, sess := range session.clients {
					if writeErr := sess.Write(event); writeErr != nil {
						log.Println(writeErr)
					}
				}

				break
			}
		}
	})()

	return session
}

func (s *Session) setupTimeout() {
	if s.timeout == nil {
		s.timeout = time.After(s.party.Settings.Timeout)
	}
}

func (s *Session) GetQueue() *Queue {
	return s.queue
}

func (s *Session) ClientConnected(client *melody.Session) {
	attendee_count := len(s.clients)
	log.Println("Connected to party with", attendee_count, "other active attendees")

	user_id := client.MustGet("user_id").(string)

	res, _ := json.Marshal(gin.H{
		"type": "attendee.active",
		"user": user_id,
	})

	for _, sess := range s.clients {
		if writeErr := sess.Write(res); writeErr != nil {
			log.Println(writeErr)
		}
	}

	s.clients[user_id] = client
}

func (s *Session) ClientDisconnected(client *melody.Session) {
	user_id := client.MustGet("user_id").(string)
	delete(s.clients, user_id)

	attendee_count := len(s.clients)
	log.Println("Left session with", attendee_count, "other active attendees")

	// Notify others this attendee has disconnected
	res, _ := json.Marshal(gin.H{
		"type": "attendee.offline",
		"user": user_id,
	})

	for _, sess := range s.clients {
		if writeErr := sess.Write(res); writeErr != nil {
			log.Println(writeErr)
		}
	}
}

func (s *Session) Push(item models.Item) error {
	conn, err := s.redis.GetConnection()
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := s.queue.Push(conn, s.party.ID.Hex(), item); err != nil {
		return err
	}

	event, err := json.Marshal(gin.H{
		"queue": s.queue,
		"type":  "queue.change",
	})

	if err != nil {
		return err
	}

	for _, sess := range s.clients {
		if writeErr := sess.Write(event); writeErr != nil {
			log.Println(writeErr)
		}
	}

	return nil
}

func (s *Session) Stop() {

	//s.Inactive <- true
}

func (s *Session) Pause() (error) {
	if s.state == INTERRUPTED {
		return Interrupted
	} else if s.CurrentPlayer != nil && s.CurrentPlayer.HasItems() {
		if err := s.CurrentPlayer.Pause(); err != nil {
			return err
		} else {
			s.state = PAUSED
		}
	} else {
		return EmptyQueue
	}

	return nil
}

func (s *Session) Next() (error) {
	//if s.CurrentPlayer != nil {
	//	if !s.CurrentPlayer.HasItems() {
	//		items := s.queue.GetNextPlayableList()
	//		if len(items) > 0 {
	//			player, err := s.GetPlayerForItem(items[0])
	//			if err != nil {
	//				return err
	//			} else if player != s.CurrentPlayer {
	//				s.CurrentPlayer = player
	//			}
	//
	//			return s.CurrentPlayer.Play(items)
	//		} else {
	//			s.CurrentPlayer = nil
	//		}
	//	} else {
	//		return s.CurrentPlayer.Next()
	//	}
	//}
	panic("NOT IMPLEMENTED")

	return nil
}

func (s *Session) Play() (error) {
	if s.CurrentPlayer != nil {
		if !s.CurrentPlayer.HasItems() {
			items := s.queue.GetNextPlayableList()
			if len(items) > 0 {
				player, err := s.GetPlayerForItem(items[0])
				if err != nil {
					return err
				} else if player != s.CurrentPlayer {
					s.CurrentPlayer = player
				}

				log.Println(items)
				if err := s.CurrentPlayer.Play(items); err != nil {
					return err
				} else {
					s.state = PLAYING
				}
			} else {
				s.CurrentPlayer = nil
				return EmptyQueue
			}
		} else {
			switch s.state {
			case INTERRUPTED:
				log.Println("RESUMING AFTER INTERRUPTION")
				if err := s.CurrentPlayer.Play(nil); err != nil {
					return err
				}
				s.state = PLAYING
				break
			case PAUSED:
				if err := s.CurrentPlayer.Resume(); err != nil {
					return err
				}
				s.state = PLAYING
			}
		}
	} else {
		items := s.queue.GetNextPlayableList()
		if len(items) > 0 {
			player, err := s.GetPlayerForItem(items[0])
			if err != nil {
				return err
			}

			s.CurrentPlayer = player

			if err := player.Play(items); err != nil {
				return err
			}
		} else {
			return EmptyQueue
		}
	}

	return nil
}

func (s *Session) GetPlayerForItem(item models.Item) (Player, error) {
	player_type := item.GetPlayerType()
	if s.players[player_type] != nil {
		return s.players[player_type], nil
	} else {
		var (
			player Player
			err    error
		)

		switch player_type {
		case "spotify":
			token := s.party.Host.GetIdentityToken("spotify")

			if token == nil {
				log.Println("NIL TOKEN")
			}

			player, err = spotify.New(s.emitter, token, nil)
			break
		}

		if err != nil {
			return nil, err
		}

		s.players[player_type] = player

		return player, nil
	}
}

func (s *Session) GetParty() *models.Party {
	return s.party
}

func (s *Session) AttendeesChanged() error {
	event, err := json.Marshal(gin.H{
		"type":      "attendees.change",
		"attendees": s.party.Attendees,
	})

	if err != nil {
		return err
	}

	for _, sess := range s.clients {
		if writeErr := sess.Write(event); writeErr != nil {
			log.Println(writeErr)
		}
	}

	return nil
}

func (s *Session) TransferHost(to models.User) error {
	// Dispose existing players, create new instances with the new host's tokens
	// Notify the new host if they have a websocket connection
	if s.CurrentPlayer != nil {
		if err := s.CurrentPlayer.Pause(); err != nil {
			log.Println("Error pausing previous host's player", err)
		}
		s.CurrentPlayer = nil
	}

	for key := range s.players {
		delete(s.players, key)
	}

	// Notify the new host if they have a websocket connection
	if client, ok := s.clients[to.ID.Hex()]; ok {
		event, err := json.Marshal(gin.H{
			"type": "host.promotion",
			"host": to,
		})

		if err != nil {
			return err
		}

		client.Write(event)
	}

	return nil
}

func InitiateConnect(redis redis.Conn, party models.Party, attendee bson.ObjectId) (string, error) {
	token := attendee.Hex() + party.JoinCode
	hasher := sha1.New()
	hasher.Write([]byte(token))
	sha := base64.URLEncoding.EncodeToString(hasher.Sum(nil))

	if reply, err := redis.Do("GET", JOIN_CODE_PREFIX+sha); err != nil {
		return "", err
	} else if reply == nil {
		if reply, err := redis.Do("SETEX", JOIN_CODE_PREFIX+sha, 30, party.ID.Hex()); err != nil {
			return "", err
		} else if reply == "OK" {
			return sha, nil
		} else {
			return "", errors.New("failed setting connect token")
		}
	} else {
		return "", ConnectTokenIssued
	}
}

func FinishConnect(conn redis.Conn, connect_token string) (bool, string, error) {
	// Delete the connect token and get the party for this session
	conn.Send("MULTI")
	conn.Send("GET", JOIN_CODE_PREFIX+connect_token)
	conn.Send("DEL", JOIN_CODE_PREFIX+connect_token)
	reply, err := redis.Values(conn.Do("EXEC"))

	if err != nil {
		return false, "", err
	}

	var id string
	var n_deleted int64

	if _, err := redis.Scan(reply, &id, &n_deleted); err != nil {
		return false, "", err
	}

	return n_deleted == 1, id, nil
}
