package store

type NonceStorer interface {
	MarkUsed(nonce string) bool
	IsUsed(nonce string) bool
	Close()
}
