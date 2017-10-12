package controllers

//import (
//	"dubclan/api/models"
//	"github.com/gin-gonic/gin"
//	"gopkg.in/mgo.v2"
//	"dubclan/api/store"
//	"gopkg.in/mgo.v2/bson"
//)

//import "dubclan/api/models"

//func CompleteUserAuth(context *gin.Context, assume_identity models.Identity) (*models.User, error) {
//	// Request has jwt token for existing user?
//	// Yes ->
//	// 		Different provider?
//	// 		Yes -> Get existing user and save new identity
//	// 		No -> Login/Refresh Access token
//	// No ->
//	// 		Does identity's email collide with identity for an existing user of this provider?
//	// 		Yes -> Login/Refresh Access token
//	// 		No -> register new user in store
//
//	mongo_session := context.MustGet("mongo_session").(*mgo.Session)
//
//	if user_id, exists := context.Get("userID"); exists {
//		user, err := datastore.UserByID(user_id.(bson.ObjectId), mongo_session.DB("test"))
//
//		if err != nil {
//			return nil, err
//		}
//
//		if len(user.Model.Identities) > 0 {
//			new_identity := true
//			for _, identity := range user.Model.Identities {
//				if identity.Provider == assume_identity.Provider {
//					if
//				}
//			}
//		}
//		if assume_identity.Provider !=
//	} else {
//		datastore.UserByIdentity(assume_identity, mongo_session.DB(main.database))
//	}
//}
