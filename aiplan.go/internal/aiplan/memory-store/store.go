package store

type Store struct {
	EmailChange *EmailChangeStore
}

func NewStore() *Store {
	return &Store{
		EmailChange: NewEmailChangeStore(),
	}
}
