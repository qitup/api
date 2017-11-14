package controllers

import (
	"dubclan/api/store"
)

type baseController struct {
	Mongo *store.MongoStore
	Redis *store.RedisStore
}

func newBaseController(mongo *store.MongoStore, redis *store.RedisStore) baseController {
	return baseController{
		mongo,
		redis,
	}
}