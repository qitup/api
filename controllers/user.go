package controllers

import (
	"dubclan/api/models"
	"github.com/gin-gonic/gin"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"errors"
	"dubclan/api/store"
)

type UserController struct {
	baseController
}

func NewUserController(mongo *store.MongoStore, redis *store.RedisStore) UserController {
	return UserController{
		newBaseController(mongo, redis),
	}
}

// Request has jwt token for existing user?
// Yes ->
// 		Different provider?
// 		Yes -> Get existing user and save new identity
// 		No -> Login/Refresh Access token
// No ->
// 		Does identity's email collide with identity for an existing user of this provider?
// 		Yes -> Login/Refresh Access token
// 		No -> register new user in store
func (c *UserController) CompleteUserAuth(context *gin.Context, assume_identity models.Identity) (*models.User, error) {
	session, db := c.Mongo.DB()
	defer session.Close()

	if user_id, exists := context.Get("userID"); exists {
		id := bson.ObjectIdHex(user_id.(string))
		if err := models.UpdateIdentityById(db, id, assume_identity); err == nil {
			return &models.User{ID: id}, nil
		} else {
			return nil, err
		}
	} else {
		if user, err := models.UpdateUserByIdentity(db, assume_identity); err == nil {
			return user, nil
		} else if err == mgo.ErrNotFound {
			new_user := &models.User{
				ID:         bson.NewObjectId(),
				Identities: []*models.Identity{&assume_identity},
			}

			if err := new_user.AssumeIdentity(db, assume_identity); err == nil {
				return new_user, nil
			} else {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
}

func (c *UserController) Me(context *gin.Context) {
	session, db := c.Mongo.DB()
	defer session.Close()

	if user_id, exists := context.Get("userID"); exists {

		if user, err := models.UserByID(db, bson.ObjectIdHex(user_id.(string))); err == nil {
			context.JSON(200, user)
		} else {
			context.AbortWithError(500, err)
		}
	} else {
		context.AbortWithError(500, errors.New("user id is nil somehow..."))
	}
}
