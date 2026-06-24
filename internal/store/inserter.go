package store

// Inserter is the generic interface for entity inserters that hold prepared statements.
// T is the parsed domain type (e.g. parse.Artist).
type Inserter[T any] interface {
	Insert(*T) error
	Close() error
}
