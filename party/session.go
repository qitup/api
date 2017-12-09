package party

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"dubclan/api/models"
	"dubclan/api/player"
	"dubclan/api/player/spotify"
	"dubclan/api/store"

	"github.com/garyburd/redigo/redis"
	"github.com/gin-gonic/gin"
	"github.com/olahol/melody"
	"github.com/olebedev/emitter"
	"gopkg.in/mgo.v2/bson"
)

const (
	QueuePrefix    = "queue:"
	JoinCodePrefix = "join_code:"
)

var (
	EmptyQueue = errors.New("empty queue")

	Interrupted = errors.New("playback interrupted")

	ConnectTokenIssued = errors.New("connect token is issued for this user")
)

type Session struct {
	mongo         *store.MongoStore
	redis         *store.RedisStore
	party         *models.Party
	clients       map[string]*melody.Session
	queue         *Queue
	players       map[string]player.Player
	CurrentPlayer player.Player
	emitter       *emitter.Emitter
	timeout       *time.Timer
	timeoutMutex  sync.Mutex

	stop    chan bool
	waiter  sync.WaitGroup
	onClose func(id string)
}

func NewSession(party *models.Party, queue *Queue, mongo *store.MongoStore, redisStore *store.RedisStore, onClose func(id string)) (*Session) {
	session := &Session{
		mongo:        mongo,
		redis:        redisStore,
		party:        party,
		clients:      make(map[string]*melody.Session),
		queue:        queue,
		players:      make(map[string]player.Player),
		emitter:      emitter.New(10),
		stop:         make(chan bool),
		onClose:      onClose,
		timeoutMutex: sync.Mutex{},
	}

	session.setupTimeout()

	session.waiter.Add(1)
	go (func(stop <-chan bool) {
		change := session.emitter.On("player.track_finished")
		play := session.emitter.On("player.play")
		interrupt := session.emitter.On("player.interrupted")
		pause := session.emitter.On("player.pause")
		for {
			select {
			case _, ok := <-change:
				if ok {
					log.Println("CHANGE")
					conn, err := redisStore.GetConnection()

					if err != nil {
						panic(err)
					}

					_, err = session.queue.Pop(conn, session.party.ID.Hex())
					conn.Close()

					if session.Play() != nil {
						session.setupTimeout()
					} else {
						session.UpdateHead()
					}

					event, err := json.Marshal(gin.H{
						"queue": session.queue,
						"type":  "queue.change",
					})

					session.writeToClients(event)
				}
				break
			case _, ok := <-interrupt:
				if ok {
					session.setupTimeout()

					log.Println("INTERRUPT")
					event, _ := json.Marshal(map[string]interface{}{
						"type": "player.interrupted",
					})

					session.writeToClients(event)
				}
				break

			case _, ok := <-play:
				if ok && len(session.queue.Items) > 0 {
					session.queue.Items[0].Play()
					log.Println("PLAY")

					session.UpdateHead()

					event, _ := json.Marshal(map[string]interface{}{
						"type": "player.play",
					})

					session.writeToClients(event)
				}
				break

			case _, ok := <-pause:
				if ok && len(session.queue.Items) > 0 {
					session.queue.Items[0].Pause()
					log.Println("PAUSED")

					session.UpdateHead()

					event, _ := json.Marshal(map[string]interface{}{
						"type": "player.pause",
					})

					session.writeToClients(event)
				}
				break

			case <-stop:
				session.waiter.Done()
				return
			}
		}
	})(session.stop)

	return session
}

func (s *Session) writeToClients(msg []byte) {
	for id, client := range s.clients {
		if writeErr := client.Write(msg); writeErr != nil {
			if writeErr.Error() == "session is closed" {
				delete(s.clients, id)
			} else {
				log.Println(writeErr)
			}
		}
	}
}

func (s *Session) setupTimeout() {
	s.timeoutMutex.Lock()
	if s.timeout == nil {
		s.timeout = time.AfterFunc(time.Second*s.party.Settings.Timeout, func() {
			log.Println("Party timedout")
			s.Close()
		})
		log.Println("Setup timeout")
	}
	s.timeoutMutex.Unlock()
}

func (s *Session) clearTimeout() {
	s.timeoutMutex.Lock()
	if s.timeout != nil {
		s.timeout.Stop()
		s.timeout = nil
		log.Println("Cleared timeout")
	}
	s.timeoutMutex.Unlock()
}

func (s *Session) GetQueue() *Queue {
	return s.queue
}

func (s *Session) ClientConnected(client *melody.Session) {
	attendeeCount := len(s.clients)
	log.Println("Connected to party with", attendeeCount, "other active attendees")

	userId := client.MustGet("user_id").(string)

	event, _ := json.Marshal(gin.H{
		"type": "attendee.active",
		"user": userId,
	})

	s.writeToClients(event)

	s.clients[userId] = client
}

func (s *Session) ClientDisconnected(client *melody.Session) {
	userId := client.MustGet("user_id").(string)
	delete(s.clients, userId)

	attendeeCount := len(s.clients)
	log.Println("Left session with", attendeeCount, "other active attendees")

	// Notify others this attendee has disconnected
	event, _ := json.Marshal(gin.H{
		"type": "attendee.offline",
		"user": userId,
	})

	s.writeToClients(event)
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

	s.writeToClients(event)

	return nil
}

func (s *Session) Close() {
	// Unsubscribe emitter listeners
	s.emitter.Off("*")
	// Signal goroutines to stop
	close(s.stop)

	s.waiter.Wait()

	if s.CurrentPlayer != nil {
		s.CurrentPlayer.Pause()
	}

	for _, p := range s.players {
		p.Stop()
	}

	conn, err := s.redis.GetConnection()
	if err == nil {
		s.queue.Delete(conn, s.party.ID.Hex())
		conn.Close()
	}

	session, db := s.mongo.DB()
	s.party.Remove(db)
	session.Close()

	event, _ := json.Marshal(gin.H{
		"type": "party.close",
	})

	for _, client := range s.clients {
		if writeErr := client.Write(event); writeErr != nil {
			log.Println(writeErr)
		}

		client.Close()
	}

	s.onClose(s.party.ID.Hex())
}

func (s *Session) Pause() (error) {
	if s.CurrentPlayer != nil {
		if s.CurrentPlayer.GetState() == player.INTERRUPTED {
			return Interrupted
		} else if s.CurrentPlayer.HasItems() {
			if err := s.CurrentPlayer.Pause(); err != nil {
				return err
			}

			s.UpdateHead()
			s.setupTimeout()
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

func (s *Session) UpdateHead() (error) {
	if len(s.queue.Items) > 0 {
		conn, err := s.redis.GetConnection()

		if err != nil {
			panic(err)
		}
		s.queue.UpdateHead(conn, s.party.ID.Hex())
		conn.Close()
	}
	return nil
}

func (s *Session) Play() (error) {
	if s.CurrentPlayer != nil {
		if !s.CurrentPlayer.HasItems() {
			items := s.queue.GetNextPlayableList()
			if len(items) > 0 {
				p, err := s.GetPlayerForItem(items[0])
				if err != nil {
					return err
				} else if p != s.CurrentPlayer {
					s.CurrentPlayer = p
				}

				if err := s.CurrentPlayer.Play(items); err != nil {
					return err
				}

				s.UpdateHead()
			} else {
				s.CurrentPlayer = nil
				return EmptyQueue
			}
		} else {
			if err := s.CurrentPlayer.Play(nil); err != nil {
				return err
			}

			s.UpdateHead()
		}
	} else {
		items := s.queue.GetNextPlayableList()
		if len(items) > 0 {
			p, err := s.GetPlayerForItem(items[0])
			if err != nil {
				return err
			}

			s.CurrentPlayer = p

			if err := p.Play(items); err != nil {
				return err
			}

			s.UpdateHead()
		} else {
			return EmptyQueue
		}
	}
	s.clearTimeout()

	return nil
}

func (s *Session) GetPlayerForItem(item models.Item) (player.Player, error) {
	playerType := item.GetPlayerType()
	if s.players[playerType] != nil {
		return s.players[playerType], nil
	} else {
		var (
			p   player.Player
			err error
		)

		switch playerType {
		case "spotify":
			token := s.party.Host.GetIdentityToken("spotify")

			if token == nil {
				log.Println("NIL TOKEN")
			}

			p, err = spotify.New(s.emitter, token, nil)
			break
		}

		if err != nil {
			return nil, err
		}

		s.players[playerType] = p

		return p, nil
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

	s.writeToClients(event)

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

	if reply, err := redis.Do("GET", JoinCodePrefix+sha); err != nil {
		return "", err
	} else if reply == nil {
		if reply, err := redis.Do("SETEX", JoinCodePrefix+sha, 30, party.ID.Hex()); err != nil {
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

func FinishConnect(conn redis.Conn, connectToken string) (bool, string, error) {
	// Delete the connect token and get the party for this session
	conn.Send("MULTI")
	conn.Send("GET", JoinCodePrefix+connectToken)
	conn.Send("DEL", JoinCodePrefix+connectToken)
	reply, err := redis.Values(conn.Do("EXEC"))

	if err != nil {
		return false, "", err
	}

	var id string
	var nDeleted int64

	if _, err := redis.Scan(reply, &id, &nDeleted); err != nil {
		return false, "", err
	}

	return nDeleted == 1, id, nil
}
