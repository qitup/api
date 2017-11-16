package party

import (
	"github.com/olahol/melody"
	"github.com/garyburd/redigo/redis"
	"crypto/sha1"
	"dubclan/api/models"
	"errors"
	"encoding/base64"
	//"github.com/VividCortex/multitick"
	"dubclan/api/party/spotify"
	"golang.org/x/oauth2"
)

const (
	PARTY_PREFIX     = "party:"
	JOIN_CODE_PREFIX = "join_code:"
)

var ConnectTokenIssued = errors.New("connect token is issued for this user")

type Session struct {
	Host     models.User
	Sessions map[*melody.Session]*melody.Session
	Queue    *Queue
	Players  map[string]Player
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

func (s *Session) Pause() {
	if player, ok := s.Players["spotify"]; ok {
		player.Pause()
	} else {
		var player Player

		player = spotify.New(
			&oauth2.Token{
				AccessToken: s.Host.GetIdentity("spotify").AccessToken,
				TokenType:   "Bearer",
			}, nil)

		s.Players["spotify"] = player
		player.Pause()
	}
}

func (s *Session) Play() {
	if player, ok := s.Players["spotify"]; ok {
		player.Play(nil)
	} else {
		var player Player

		player = spotify.New(
			&oauth2.Token{
				AccessToken: s.Host.GetIdentity("spotify").AccessToken,
				TokenType:   "Bearer",
			}, nil)

		s.Players["spotify"] = player
		player.Play(nil)
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
