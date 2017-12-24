package party

import (
	"encoding/json"
	"log"

	"dubclan/api/models"

	"github.com/garyburd/redigo/redis"
)

type Queue struct {
	Items []models.Item `json:"items" bson:"items"`
}

func NewQueue() *Queue {
	return &Queue{
		Items: []models.Item{},
	}
}

func ResumeQueue(conn redis.Conn, id string) (*Queue, error) {
	if list, err := redis.Strings(conn.Do("LRANGE", QueuePrefix+id, 0, -1)); err == nil {
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
	firstType := ""
	var items []models.Item
	for _, item := range q.Items {
		if firstType == "" {
			firstType = item.GetType()
			items = append(items, item)
		} else {
			if itemType := item.GetType(); itemType == firstType {
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
		_, err := conn.Do("LPUSH", QueuePrefix+id, serialized)

		if err == nil {
			q.Items = append(q.Items, item)
		}

		return err
	} else {
		return err
	}
}

func (q *Queue) Pop(conn redis.Conn, id string) (models.Item, error) {
	if raw, err := redis.String(conn.Do("RPOP", QueuePrefix+id)); err == nil {
		_, q.Items = q.Items[0], q.Items[1:]

		u := &models.ItemUnpacker{}
		if err := json.Unmarshal([]byte(raw), u); err == nil {
			return u.Result, nil
		} else {
			return nil, err
		}
	} else {
		return nil, err
	}
}

func (q *Queue) UpdateHead(conn redis.Conn, id string) error {
	if serialized, err := json.Marshal(q.Items[0]); err == nil {
		_, err := conn.Do("LSET", QueuePrefix+id, -1, serialized)

		return err
	} else {
		return err
	}
}

func (q *Queue) Delete(conn redis.Conn, id string) error {
	_, err := conn.Do("DEL", QueuePrefix+id)

	return err
}
