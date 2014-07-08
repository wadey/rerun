package main

type Watcher interface {
	Chan() <-chan Event
	Close() error
}

type Event struct {
	Name string
}
