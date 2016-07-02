// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build windows

package main

import (
	"fmt"
	"time"

	"net/http"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"

	"os"
)

var elog debug.Log

type myservice struct{}

func (m *myservice) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown

	changes <- svc.Status{State: svc.StartPending}
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	go func() {
		elog.Info(1, "Starting http server v2")
		dir := os.Getenv("GO_HTTP_ROOT")

		// Maybe make this configurable
		file := dir + string(os.PathSeparator) + "access.log"
		elog.Info(1, fmt.Sprintf("Opening http log file:s", file))

		f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE, 0666)
		if err != nil {
			elog.Error(1, fmt.Sprintf("There was an error opening the log file: %s.", err.Error()))
			os.Exit(1)
		}
		defer f.Close()

		f.WriteString("server started\n")

		fileHandler := http.FileServer(http.Dir(dir))
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			f.WriteString(fmt.Sprintf("%s %s %s\n", r.RemoteAddr, r.Method, r.URL))
			fileHandler.ServeHTTP(w, r)
		})

		e := http.ListenAndServe(":80", nil)

		elog.Info(1, e.Error())
	}()

loop:
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
				// Testing deadlock from https://code.google.com/p/winsvc/issues/detail?id=4
				time.Sleep(100 * time.Millisecond)
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				break loop
			case svc.Pause:
				changes <- svc.Status{State: svc.Paused, Accepts: cmdsAccepted}
			case svc.Continue:
				changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
			default:
				elog.Error(1, fmt.Sprintf("unexpected control request #%d", c))
			}
		}
	}
	changes <- svc.Status{State: svc.StopPending}
	os.Exit(0)
	return
}

func runService(name string, isDebug bool) {
	var err error
	if isDebug {
		elog = debug.New(name)
	} else {
		elog, err = eventlog.Open(name)
		if err != nil {
			return
		}
	}
	defer elog.Close()

	elog.Info(1, fmt.Sprintf("starting %s service", name))
	run := svc.Run
	if isDebug {
		run = debug.Run
	}
	err = run(name, &myservice{})
	if err != nil {
		elog.Error(1, fmt.Sprintf("%s service failed: %v", name, err))
		return
	}
	elog.Info(1, fmt.Sprintf("%s service stopped", name))
}
