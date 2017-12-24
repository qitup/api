package controllers

import (
	"errors"
	"strings"

	"dubclan/api/models"
	"dubclan/api/store"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/dgrijalva/jwt-go.v3"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type UserController struct {
	baseController
	key []byte
}

func NewUserController(mongo *store.MongoStore, redis *store.RedisStore, key []byte) UserController {
	return UserController{
		baseController: newBaseController(mongo, redis),
		key:            key,
	}
}

func (c *UserController) jwtFromHeader(context *gin.Context, key string) (string, error) {
	authHeader := context.Request.Header.Get(key)

	if authHeader == "" {
		return "", errors.New("auth header empty")
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if !(len(parts) == 2 && parts[0] == "Bearer") {
		return "", errors.New("invalid auth header")
	}

	return parts[1], nil
}

func (c *UserController) parseToken(context *gin.Context) (*jwt.Token, error) {
	var token string
	var err error

	token, err = c.jwtFromHeader(context, "Authorization")

	if err != nil {
		return nil, err
	}

	return jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		if jwt.GetSigningMethod("HS256") != token.Method {
			return nil, errors.New("invalid signing algorithm")
		}

		return c.key, nil
	})
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
func (c *UserController) CompleteUserAuth(context *gin.Context, assumeIdentity models.Identity) (*models.User, error) {
	session, db := c.Mongo.DB()
	defer session.Close()

	if token, err := c.parseToken(context); err == nil {
		claims := token.Claims.(jwt.MapClaims)
		id := bson.ObjectIdHex(claims["sub"].(string))

		if err := models.UpdateUserIdentity(db, id, assumeIdentity); err == nil {
			return &models.User{ID: id}, nil
		} else {
			return nil, err
		}
	} else {
		if user, err := models.UpdateUserByIdentity(db, assumeIdentity); err == nil {
			return user, nil
		} else if err == mgo.ErrNotFound {
			newUser := &models.User{
				ID:         bson.NewObjectId(),
				Identities: []*models.Identity{&assumeIdentity},
				CanHost:    assumeIdentity.Provider == "spotify",
			}

			if err := newUser.AssumeIdentity(db, assumeIdentity); err == nil {
				return newUser, nil
			} else {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
}

func (c *UserController) Signup(context *gin.Context, host string) {
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

	newUser := &models.User{
		ID:       bson.NewObjectId(),
		CanHost:  false,
		Email:    fields.Email,
		Username: fields.Username,
		Password: hash,
	}

	// Ensure account doesnt exist
	if err := newUser.Insert(db); err != nil {
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

	token, err := newUser.NewToken(host, c.key)
	if err != nil {
		context.AbortWithError(500, err)
	}

	context.JSON(200, bson.M{
		"token": token,
	})
}

func (c *UserController) Login(context *gin.Context, host string) {
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
	tokenString, err := user.NewToken(host, c.key)

	if err != nil {
		context.AbortWithError(500, err)
		return
	}

	context.JSON(200, gin.H{
		"token": tokenString,
	})
}

func (c *UserController) Me(context *gin.Context) {
	session, db := c.Mongo.DB()
	defer session.Close()

	if userId, exists := context.Get("userID"); exists {

		if user, err := models.UserByID(db, bson.ObjectIdHex(userId.(string))); err == nil {
			context.JSON(200, user)
		} else {
			context.AbortWithError(500, err)
		}
	} else {
		context.AbortWithError(500, errors.New("user id is nil"))
	}
}
