package party

import (
	"github.com/garyburd/redigo/redis"
	"encoding/json"
	"log"
	"dubclan/api/models"
)

type State struct {
	cursor      int
	currentItem *models.Item
}
type Queue struct {
	Items []models.Item `json:"items" bson:"items"`
	State State
}

func NewQueue() *Queue {
	return &Queue{
		Items: []models.Item{},
	}
}

func ResumeQueue(conn redis.Conn, id string) (*Queue, error) {
	if list, err := redis.Strings(conn.Do("LRANGE", QUEUE_PREFIX+id, 0, -1)); err == nil {
		queue := NewQueue()

		u := &models.ItemUnpacker{}
		for i := len(list) - 1; i >= 0; i-- {
			if err := json.Unmarshal([]byte(list[i]), u); err == nil {
				queue.Items = append(queue.Items, u.Result)
			} else {
				log.Println(err)
				return nil, err
			}
		}

		return queue, nil
	} else {
		return nil, err
	}
}

func (q *Queue) GetNextPlayableList() []models.Item {
	first_type := ""
	items := []models.Item{}
	for _, item := range q.Items {
		if first_type == "" {
			first_type = item.GetType()
			items = append(items, item)
		} else {
			if item_type := item.GetType(); item_type == first_type {
				items = append(items, item)
			} else {
				break
			}
		}
	}
	return items
}

func (q *Queue) Push(conn redis.Conn, id string, item models.Item) error {
	if serialized, err := json.Marshal(item); err == nil {
		_, err := conn.Do("LPUSH", QUEUE_PREFIX+id, serialized)

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
