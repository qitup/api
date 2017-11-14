package store

import "gopkg.in/mgo.v2"

type MongoStore struct {
	DataStore
	session *mgo.Session
	database string
}

func NewMongoStore(session *mgo.Session, database string) *MongoStore {
	return &MongoStore{
		session: session,
		database: database,
	}
}

func (s *MongoStore) DB() (*mgo.Session, *mgo.Database) {
	session := s.session.Copy()
	return session, session.DB(s.database)
}
