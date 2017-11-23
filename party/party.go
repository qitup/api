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
)

const (
	QUEUE_PREFIX     = "queue:"
	JOIN_CODE_PREFIX = "join_code:"
)

var ConnectTokenIssued = errors.New("connect token is issued for this user")

type Session struct {
	mongo         *store.MongoStore
	redis         *store.RedisStore
	party         *models.Party
	clients       map[*melody.Session]*melody.Session
	queue         *Queue
	players       map[string]Player
	CurrentPlayer Player
	emitter       *emitter.Emitter
}

func NewSession(party *models.Party, queue *Queue, mongo *store.MongoStore, redis *store.RedisStore) (*Session) {
	session := &Session{
		mongo:   mongo,
		redis:   redis,
		party:   party,
		clients: make(map[*melody.Session]*melody.Session),
		queue:   queue,
		players: make(map[string]Player),
		emitter: emitter.New(10),
	}

	go (func() {
		change := session.emitter.On("player.change")
		start := session.emitter.On("player.start")
		interrupt := session.emitter.On("player.interrupt")
		pause := session.emitter.On("player.pause")
		for {
			select {
				case <-change:
					break
				case <-start:
					break
				case <-interrupt:
					break
				case <-pause:
					break
			}
		}
	})()

	return session
}

func (s *Session) GetQueue() *Queue {
	return s.queue
}

func (s *Session) ClientConnected(client *melody.Session) {
	attendee_count := len(s.clients)
	log.Println("Connected to party with", attendee_count, "other active attendees")

	res, _ := json.Marshal(gin.H{
		"type": "attendee.active",
		"user": client.MustGet("user_id"),
	})

	for _, sess := range s.clients {
		if writeErr := sess.Write(res); writeErr != nil {
			log.Println(writeErr)
		}
	}

	s.clients[client] = client
}

func (s *Session) ClientDisconnected(client *melody.Session) {
	delete(s.clients, client)

	attendee_count := len(s.clients)
	log.Println("Left session with", attendee_count, "other active attendees")

	// Notify others this attendee has disconnected
	res, _ := json.Marshal(gin.H{
		"type": "attendee.offline",
		"user": client.MustGet("user_id"),
	})

	for _, sess := range s.clients {
		if writeErr := sess.Write(res); writeErr != nil {
			log.Println(writeErr)
		}
	}
}

func (s *Session) Push(client *melody.Session, item models.Item) error {
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
	if s.CurrentPlayer != nil && s.CurrentPlayer.HasItems() {
		return s.CurrentPlayer.Pause()
	}

	return nil
}

func (s *Session) Next() (error) {
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

				return s.CurrentPlayer.Play(items)
			} else {
				s.CurrentPlayer = nil
			}
		} else {
			return s.CurrentPlayer.Next()
		}
	}

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

				return s.CurrentPlayer.Play(items)
			} else {
				s.CurrentPlayer = nil
			}
		} else {
			return s.CurrentPlayer.Resume()
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
			} else {
				s.queue.State.currentItem = &items[0]
				s.queue.State.cursor = 0
			}
		} else {
			s.CurrentPlayer = nil
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
			identity := s.party.Host.GetRefreshableIdentity("spotify", s.mongo)

			player, err = spotify.New(s.emitter, identity, nil)
			break
		}

		if err != nil {
			return nil, err
		}

		s.players[player_type] = player

		return player, nil
	}
}

func (s *Session) TransferHost(to bson.ObjectId) {

}

func InitiateConnect(redis redis.Conn, party models.Party, attendee models.Attendee) (string, error) {
	token := attendee.UserId.Hex() + party.JoinCode
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
