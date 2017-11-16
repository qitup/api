package party

import (
	"github.com/olahol/melody"
	"github.com/garyburd/redigo/redis"
	"crypto/sha1"
	"dubclan/api/models"
	"errors"
	"encoding/base64"
	//"github.com/VividCortex/multitick"
	"time"
	"log"
)

const (
	PARTY_PREFIX     = "party:"
	JOIN_CODE_PREFIX = "join_code:"
)

var ConnectTokenIssued = errors.New("connect token is issued for this user")

type Session struct {
	Host models.User
	Sessions map[*melody.Session]*melody.Session
	Queue    *Queue
	Players map[string]Player
}

func NewSession(queue *Queue) (*Session) {
	session := &Session{
		Sessions: make(map[*melody.Session]*melody.Session),
		Queue:    queue,
	}

	return session
}

func (s *Session) InitializePlayer(service string, player Player) {

	s.Players[service] = player
}

func (s *Session) Play(player_type string) {
	p := s.Players[player_type]

	items := s.Queue.GetNextPlayableList()


}

func (s *Session) Stop() {
	//s.Inactive <- true
}

// Update the state of the players until a session becomes inactive
func (s *Session) update(ticked <-chan time.Time) {
	for {
		select {
		// Update states of our players
		case tick := <-ticked:
			log.Println("TICKED", tick)
			//for _, player := range s.Players {
			//	player.UpdateState()
			//}
			break

		//case done := <-s.Inactive:
		//	if done {
		//		return
		//	}
		//	break
		}
	}
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
