// +build !darwin

package main

import (
	"go/build"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

type fsnotifyWatcher struct {
	watcher *fsnotify.Watcher
	ch      chan Event
}

func (w *fsnotifyWatcher) Chan() <-chan Event {
	return w.ch
}

func (w *fsnotifyWatcher) Close() (err error) {
	if w.ch != nil {
		close(w.ch)
		// close the watcher
		err = w.watcher.Close()
		// to clean things up: read events from the watcher until events chan is closed.
		go func(events chan *fsnotify.FileEvent) {
			for _ = range events {
			}
		}(w.watcher.Event)
	}

	return
}

func getWatcher(buildpath string) (Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	addToWatcher(fsw, buildpath, map[string]bool{})
	ch := make(chan Event)
	go func() {
		for {
			// read event from the watcher
			select {

			case we := <-fsw.Event:
				// other files in the directory don't count - we watch the whole thing in case new .go files appear.
				if filepath.Ext(we.Name) != ".go" {
					continue
				}

				ch <- Event{Name: we.Name}
			case <-ch:
				return
			}

		}
	}()
	// we don't need the errors from the new watcher.
	// we continiously discard them from the channel to avoid a deadlock.
	go func(errors chan error) {
		for _ = range errors {
		}
	}(fsw.Error)
	return &fsnotifyWatcher{watcher: fsw, ch: ch}, nil
}

func addToWatcher(watcher *fsnotify.Watcher, importpath string, watching map[string]bool) {
	pkg, err := build.Import(importpath, "", 0)
	if err != nil {
		return
	}
	if pkg.Goroot {
		return
	}
	watcher.Watch(pkg.Dir)
	watching[importpath] = true
	for _, imp := range pkg.Imports {
		if !watching[imp] {
			addToWatcher(watcher, imp, watching)
		}
	}
}
