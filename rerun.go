// Copyright 2013 The rerun AUTHORS. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/build"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
)

var (
	do_tests      = flag.Bool("test", false, "Run tests (before running program)")
	do_build      = flag.Bool("build", false, "Build program")
	never_run     = flag.Bool("no-run", false, "Do not run")
	race_detector = flag.Bool("race", false, "Run program and tests with the race detector")
)

func install(buildpath, lastError string) (installed bool, errorOutput string, err error) {
	cmdline := []string{"go", "get"}

	if *race_detector {
		cmdline = append(cmdline, "-race")
	}
	cmdline = append(cmdline, buildpath)

	// setup the build command, use a shared buffer for both stdOut and stdErr
	cmd := exec.Command("go", cmdline[1:]...)
	buf := bytes.NewBuffer([]byte{})
	cmd.Stdout = buf
	cmd.Stderr = buf

	err = cmd.Run()

	// when there is any output, the go command failed.
	if buf.Len() > 0 {
		errorOutput = buf.String()
		if errorOutput != lastError {
			fmt.Print(errorOutput)
		}
		err = errors.New("compile error")
		return
	}

	// all seems fine
	installed = true
	return
}

func test(buildpath string) (passed bool, err error) {
	cmdline := []string{"go", "test"}

	if *race_detector {
		cmdline = append(cmdline, "-race")
	}
	cmdline = append(cmdline, "-v", buildpath)

	// setup the build command, use a shared buffer for both stdOut and stdErr
	cmd := exec.Command("go", cmdline[1:]...)
	buf := bytes.NewBuffer([]byte{})
	cmd.Stdout = buf
	cmd.Stderr = buf

	err = cmd.Run()
	passed = err == nil

	if !passed {
		fmt.Println(buf)
	} else {
		log.Println("tests passed")
	}

	return
}

func gobuild(buildpath string) (passed bool, err error) {
	cmdline := []string{"go", "build"}

	if *race_detector {
		cmdline = append(cmdline, "-race")
	}
	cmdline = append(cmdline, "-v", buildpath)

	// setup the build command, use a shared buffer for both stdOut and stdErr
	cmd := exec.Command("go", cmdline[1:]...)
	buf := bytes.NewBuffer([]byte{})
	cmd.Stdout = buf
	cmd.Stderr = buf

	err = cmd.Run()
	passed = err == nil

	if !passed {
		fmt.Println(buf)
	} else {
		log.Println("build passed")
	}

	return
}

func run(binName, binPath string, args []string) (runch chan bool) {
	runch = make(chan bool)
	go func() {
		cmdline := append([]string{binName}, args...)
		var proc *os.Process
		for relaunch := range runch {
			if proc != nil {
				err := proc.Signal(os.Interrupt)
				if err != nil {
					log.Printf("error on sending signal to process: '%s', will now hard-kill the process\n", err)
					proc.Kill()
				}
				proc.Wait()
			}
			if !relaunch {
				continue
			}
			cmd := exec.Command(binPath, args...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			log.Print(cmdline)
			err := cmd.Start()
			if err != nil {
				log.Printf("error on starting process: '%s'\n", err)
			}
			proc = cmd.Process
		}
	}()
	return
}

func rerun(buildpath string, args []string) (err error) {
	log.Printf("setting up %s %v", buildpath, args)

	pkg, err := build.Import(buildpath, "", 0)
	if err != nil {
		return
	}

	if pkg.Name != "main" {
		err = errors.New(fmt.Sprintf("expected package %q, got %q", "main", pkg.Name))
		return
	}

	_, binName := path.Split(buildpath)
	var binPath string
	if gobin := os.Getenv("GOBIN"); gobin != "" {
		binPath = filepath.Join(gobin, binName)
	} else {
		binPath = filepath.Join(pkg.BinDir, binName)
	}

	var runch chan bool
	if !(*never_run) {
		runch = run(binName, binPath, args)
	}

	no_run := false
	if *do_tests {
		passed, _ := test(buildpath)
		if !passed {
			no_run = true
		}
	}

	if *do_build && !no_run {
		gobuild(buildpath)
	}

	var errorOutput string
	_, errorOutput, ierr := install(buildpath, errorOutput)
	if !no_run && !(*never_run) && ierr == nil {
		runch <- true
	}

	watcher, err := getWatcher(buildpath)
	if err != nil {
		return
	}

	for {
		// read event from the watcher
		we, _ := <-watcher.Chan()

		log.Print(we)

		watcher.Close()
		// create a new watcher
		log.Println("rescanning")
		watcher, err = getWatcher(buildpath)
		if err != nil {
			return
		}

		var installed bool
		// rebuild
		installed, errorOutput, _ = install(buildpath, errorOutput)
		if !installed {
			continue
		}

		if *do_tests {
			passed, _ := test(buildpath)
			if !passed {
				continue
			}
		}

		if *do_build {
			gobuild(buildpath)
		}

		// rerun. if we're only testing, sending
		if !(*never_run) {
			runch <- true
		}
	}
	return
}

func main() {
	flag.Parse()

	if len(flag.Args()) < 1 {
		log.Fatal("Usage: rerun [--test] [--no-run] [--build] [--race] <import path> [arg]*")
	}

	buildpath := flag.Args()[0]
	args := flag.Args()[1:]
	err := rerun(buildpath, args)
	if err != nil {
		log.Print(err)
	}
}
