package store

import (
	"github.com/gin-gonic/gin"
	"gopkg.in/mgo.v2"
	"github.com/urfave/cli"
)

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
