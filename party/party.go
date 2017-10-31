package party

import (
	"github.com/olahol/melody"
)

type Session struct {
	Sessions map[*melody.Session]*melody.Session
	Queue    *Queue
}