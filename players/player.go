package player

type Player interface {
	Play() (error)
	Stop() (error)
	Next() (error)
	Previous() (error)
	UpdateState(event Event)
}

type Event struct {
	Type     string                 `json:"type"`
	Name     string                 `json:"name"`
	Metadata map[string]interface{} `json:"metadata"`
}
