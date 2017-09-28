package middleware

import (
	"github.com/gin-gonic/gin"
	"gopkg.in/mgo.v2"
)

func Store(session *mgo.Session) gin.HandlerFunc {
	return func(context *gin.Context) {
		// copy the database session
		new_session := session.Copy()

		defer new_session.Close()

		context.Set("session", new_session)

		context.Next()
	}
}
