package party

import "dubclan/api/models"


type Player interface {
	Play(item []models.Item) (error)
	Pause() (error)
	Next() (error)
	Previous() (error)
	UpdateState() (error)
}

type Event struct {
	Type     string                 `json:"type"`
	Name     string                 `json:"name"`
	Metadata map[string]interface{} `json:"metadata"`
}
