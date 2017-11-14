package party

import (
	"dubclan/api/channels"
)

func init()  {
	channels.Mutli.Register("party", Channel{})
}

type Channel struct {
	channels.Channel
}