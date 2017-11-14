package store

import (
	"github.com/garyburd/redigo/redis"
	"github.com/gin-contrib/sessions"
	"time"
)

type RedisStore struct {
	DataStore
	pool *redis.Pool
}

func NewRedisStore(size int, network, address, password string) *RedisStore {
	return &RedisStore{
		pool: getRedisPool(size, network, address, password),
	}
}

func (s *RedisStore) GetConnection() (redis.Conn, error) {
	conn := s.pool.Get()

	if err := conn.Err(); err != nil {
		return nil, err
	}

	return conn, nil
}

func (s *RedisStore) GetSessionStore(secret []byte) (sessions.RedisStore, error) {
	return sessions.NewRedisStoreWithPool(s.pool, secret)
}

func getRedisPool(size int, network, address, password string) *redis.Pool {
	return &redis.Pool{
		MaxIdle:     size,
		IdleTimeout: 240 * time.Second,
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
		Dial: func() (redis.Conn, error) {
			return dial(network, address, password)
		},
	}
}

func dial(network, address, password string) (redis.Conn, error) {
	c, err := redis.Dial(network, address)
	if err != nil {
		return nil, err
	}
	if password != "" {
		if _, err := c.Do("AUTH", password); err != nil {
			c.Close()
			return nil, err
		}
	}
	return c, err
}