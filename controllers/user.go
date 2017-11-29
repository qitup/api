package controllers

import (
	"dubclan/api/models"
	"github.com/gin-gonic/gin"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"errors"
	"dubclan/api/store"
	"golang.org/x/crypto/bcrypt"
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
		if err := models.UpdateUserIdentity(db, id, assume_identity); err == nil {
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
				CanHost:    assume_identity.Provider == "spotify",
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

func (c *UserController) Signup(context *gin.Context, host string, key []byte) {
	var fields struct {
		Email    string `json:"email" binding:"required"`
		Password string `json:"password" binding:"required"`
		Username string `json:"username" binding:"required"`
	}

	if context.BindJSON(&fields) != nil {
		context.JSON(400, gin.H{
			"code":    400,
			"message": "Missing required signup fields",
		})
		return
	}

	session, db := c.Mongo.DB()
	defer session.Close()

	hash, err := bcrypt.GenerateFromPassword([]byte(fields.Password), bcrypt.DefaultCost)
	if err != nil {
		context.AbortWithError(500, err)
	}

	new_user := &models.User{
		ID:       bson.NewObjectId(),
		CanHost:  false,
		Email:    fields.Email,
		Username: fields.Username,
		Password: hash,
	}

	// Ensure account doesnt exist
	if err := new_user.Insert(db); err != nil {
		if mgo.IsDup(err) {
			context.JSON(400, gin.H{
				"code":    400,
				"message": "Account already exists",
			})
			return
		}
		context.AbortWithError(500, err)
		return
	}

	token, err := new_user.NewToken(host, key)
	if err != nil {
		context.AbortWithError(500, err)
	}

	context.JSON(200, bson.M{
		"token": token,
	})
}

func (c *UserController) Login(context *gin.Context, host string, key []byte) {
	var loginVals struct {
		Email    string `json:"email" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if context.BindJSON(&loginVals) != nil {
		context.JSON(400, gin.H{"code": 400, "message": "Missing Username or Password"})
		return
	}

	session, db := c.Mongo.DB()
	defer session.Close()

	user, err := models.Authenticate(db, loginVals.Email, []byte(loginVals.Password))

	if err != nil {
		if err == bcrypt.ErrMismatchedHashAndPassword || err == mgo.ErrNotFound {
			context.JSON(401, gin.H{"code": 401, "message": "Incorrect Email / Password"})
			return
		}

		context.AbortWithError(500, err)
		return
	}

	// Create the token
	tokenString, err := user.NewToken(host, key)

	if err != nil {
		context.AbortWithError(500, err)
		return
	}

	context.JSON(200, gin.H{
		"token":  tokenString,
	})
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
