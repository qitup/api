package player

type Player interface {
	Play() (error)
	Stop() (error)
	Next() (error)
	Previous() (error)
}

