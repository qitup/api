package party

import (
	"github.com/garyburd/redigo/redis"
	"encoding/json"
	"log"
)

const (
	PARTY_PREFIX = "p:"
)

type Queue struct {
	Items []Item `json:"items" bson:"items"`
}

func NewQueue() Queue {
	return Queue{
		Items: []Item{},
	}
}

func TryResumeQueue(conn redis.Conn, party string) (*Queue, error) {
	if list, err := redis.Strings(conn.Do("LRANGE", PARTY_PREFIX+party, 0, -1)); err == nil {
		queue := NewQueue()

		for i := len(list) - 1; i >= 0; i-- {
			if item, err := UnmarshalItem([]byte(list[i])); err == nil {
				queue.Items = append(queue.Items, item)
			} else {
				log.Println(err)
				return nil, err
			}
		}

		return &queue, nil
	} else {
		return nil, err
	}
}

func (q *Queue) Push(redis redis.Conn, party string, item Item) error {
	if serialized, err := json.Marshal(item); err == nil {
		_, err := redis.Do("LPUSH", PARTY_PREFIX+party, serialized)
		if err == nil {
			q.Items = append(q.Items, item)
		}

		return err
	} else {
		return err
	}
}

//func (q *Queue) HasItem(cmp Item) (bool, error) {
//	for _, item := range q.Items {
//		if cmp.
//	}
//}

//func (q *Queue) Pop(redis redis.Conn, party string, item *BaseItem) error {
//	if item, err := redis.Do("RPOP", party); err == nil {
//		if serialized, err := item.Deserialize(); err == nil {
//
//		} else {
//			return nil
//		}
//	} else {
//		return err
//	}
//}
