package storage

// Crawled data storage
type Storage interface {
	Set(key string, value any) error
	Get(key string) (any, error)
	Has(key string) bool
	Delete(key string) error
}
