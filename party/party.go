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
)

const (
	QUEUE_PREFIX     = "queue:"
	JOIN_CODE_PREFIX = "join_code:"
)

var ConnectTokenIssued = errors.New("connect token is issued for this user")

type Session struct {
	Mongo         *store.MongoStore
	Redis         *store.RedisStore
	Party         *models.Party
	Sessions      map[*melody.Session]*melody.Session
	Queue         *Queue
	Players       map[string]Player
	CurrentPlayer Player
}

func NewSession(party *models.Party, queue *Queue, mongo *store.MongoStore, redis *store.RedisStore) (*Session) {
	session := &Session{
		Mongo:    mongo,
		Redis:    redis,
		Party:    party,
		Sessions: make(map[*melody.Session]*melody.Session),
		Queue:    queue,
		Players:  make(map[string]Player),
	}

	return session
}

func (s *Session) InitializePlayer(service string, player Player) {

	s.Players[service] = player
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
			items := s.Queue.GetNextPlayableList()
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
			items := s.Queue.GetNextPlayableList()
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
		items := s.Queue.GetNextPlayableList()
		if len(items) > 0 {
			player, err := s.GetPlayerForItem(items[0])
			if err != nil {
				return err
			}

			s.CurrentPlayer = player

			if err := player.Play(items); err != nil {
				return err
			} else {
				s.Queue.State.currentItem = &items[0]
				s.Queue.State.cursor = 0
			}
		} else {
			s.CurrentPlayer = nil
		}
	}

	return nil
}

func (s *Session) GetPlayerForItem(item models.Item) (Player, error) {
	player_type := item.GetPlayerType()
	if s.Players[player_type] != nil {
		return s.Players[player_type], nil
	} else {
		var (
			player Player
			err    error
		)

		switch player_type {
		case "spotify":
			identity := s.Party.Host.GetRefreshableIdentity("spotify", s.Mongo)

			player, err = spotify.New(identity, nil)
			break
		}

		if err != nil {
			return nil, err
		}

		s.Players[player_type] = player

		return player, nil
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
