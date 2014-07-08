// +build darwin

package main

import (
	"fmt"
	"go/build"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/ronbu/fsevents"
)

type fseventsWatcher struct {
	stream  *fsevents.Stream
	ch      chan Event
	quit    chan struct{}
	pathSet map[string]bool
}

func (w *fseventsWatcher) Chan() <-chan Event {
	return w.ch
}

func (w *fseventsWatcher) Close() error {
	if w.ch != nil {
		close(w.quit)
		close(w.ch)
		w.stream.Close()
	}

	return nil
}

func getWatcher(buildpath string) (Watcher, error) {
	pathSet := map[string]bool{}
	root := addToWatcher(buildpath, pathSet)
	fi, _ := os.Stat(root)
	dev := fsevents.Device(fi.Sys().(*syscall.Stat_t).Dev)
	stream := fsevents.New(dev, fsevents.NOW, 1*time.Second, fsevents.CF_FILEEVENTS, root)
	ch := make(chan Event)
	quit := make(chan struct{})
	go func() {
		for {
			// read event from the watcher
			select {

			case events := <-stream.Chan:
				// log.Printf("%#v", e)
				for _, e := range events {
					if e.Flags&fsevents.EF_ISFILE != 0 {
						if strings.HasSuffix(e.Path, ".go") {
							// TODO check pathSet
							select {
							case ch <- Event{Name: "/" + e.Path}:
							case <-quit:
								return
							}
						}
					}
				}
			case <-quit:
				return
			}

		}
	}()
	if !stream.Start() {
		return nil, fmt.Errorf("Error starting fsevents.Stream")
	}
	return &fseventsWatcher{stream: stream, ch: ch, quit: quit}, nil
}

func addToWatcher(importpath string, watching map[string]bool) (root string) {
	pkg, err := build.Import(importpath, "", 0)
	if err != nil {
		return
	}
	if pkg.Goroot {
		return
	}
	watching[importpath] = true
	root = pkg.Dir
	for _, imp := range pkg.Imports {
		if !watching[imp] {
			oRoot := addToWatcher(imp, watching)
			if oRoot != "" {
				root = commonPrefix(root, oRoot)
			}
		}
	}
	return
}

func commonPrefix(a, b string) string {
	a += "/"
	b += "/"

	m := a
	if len(a) > len(b) {
		m = b
	}
	x := 0
	for i := 0; i < len(m); i++ {
		if a[i] != b[i] {
			break
		}
		if a[i] == '/' {
			x = i
		}
	}
	return m[:x]
}
