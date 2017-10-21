package store

import (
	"github.com/gin-gonic/gin"
	"gopkg.in/mgo.v2"
	"github.com/urfave/cli"
	"github.com/garyburd/redigo/redis"
	"time"
)

func GetRedisPool(size int, network, address, password string) *redis.Pool {
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

func Middleware(session *mgo.Session, cli *cli.Context) gin.HandlerFunc {
	return func(context *gin.Context) {
		// copy the database session
		new_session := session.Copy()

		defer new_session.Close()

		db := new_session.DB(cli.String("database"))

		context.Set("mongo", db)

		context.Next()
	}
}
