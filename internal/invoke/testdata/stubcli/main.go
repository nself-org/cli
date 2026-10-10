// Command stubcli is the stand-in nself binary for internal/invoke tests.
//
// Purpose: prove what Exec and Invoke do to a child without building the real
// binary. STUBCLI_MODE selects the behaviour; STUBCLI_STDERR is written to
// stderr in every mode; STUBCLI_EXIT is the exit status.
//
//	echo (default)  print an envelope whose data holds argv, env and cwd
//	sleep           sleep 30 s (SIGTERM ends it)
//	sleep-ignore    sleep 30 s and ignore SIGTERM
//	text            print plain text on stdout
//	two             print two JSON documents
//	meta            print an envelope carrying meta.deprecations
//	raw             print STUBCLI_STDOUT verbatim
//	big             print about 34 MiB on stdout
//	stream          print three NDJSON lines
//	grandchild      start a detached copy of this stub (mode STUBCLI_GC_MODE,
//	                default sleep-ignore), write its pid to STUBCLI_PIDFILE,
//	                then sleep 30 s
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	if s := os.Getenv("STUBCLI_STDERR"); s != "" {
		fmt.Fprintln(os.Stderr, s)
	}
	exit := 0
	if v := os.Getenv("STUBCLI_EXIT"); v != "" {
		exit, _ = strconv.Atoi(v)
	}
	switch os.Getenv("STUBCLI_MODE") {
	case "sleep-ignore":
		signal.Ignore(syscall.SIGTERM)
		time.Sleep(30 * time.Second)
	case "sleep":
		time.Sleep(30 * time.Second)
	case "text":
		fmt.Println("hello, this is not JSON")
	case "two":
		fmt.Println(`{"schema_version":"1","command":"stub","data":1}`)
		fmt.Println(`{"schema_version":"1","command":"stub","data":2}`)
	case "meta":
		fmt.Print(`{"schema_version":"1","command":"stub","data":{"ok":true},"meta":{"deprecations":[{"old":"a b","new":"c d","removal_at":"v1.6.0"}]}}` + "\n")
	case "raw":
		fmt.Print(os.Getenv("STUBCLI_STDOUT"))
	case "big":
		chunk := strings.Repeat("a", 1<<20)
		for i := 0; i < 34; i++ {
			fmt.Print(chunk)
		}
	case "grandchild":
		gc := exec.Command(os.Args[0])
		mode := os.Getenv("STUBCLI_GC_MODE")
		if mode == "" {
			mode = "sleep-ignore"
		}
		gc.Env = append(os.Environ(), "STUBCLI_MODE="+mode, "STUBCLI_STDERR=")
		if err := gc.Start(); err == nil {
			_ = os.WriteFile(os.Getenv("STUBCLI_PIDFILE"), []byte(strconv.Itoa(gc.Process.Pid)), 0o600)
		}
		time.Sleep(30 * time.Second)
	case "stream":
		for i := 1; i <= 3; i++ {
			fmt.Printf("{\"n\":%d}\n", i)
		}
	default:
		env := map[string]string{}
		for _, kv := range os.Environ() {
			if k, v, ok := strings.Cut(kv, "="); ok {
				env[k] = v
			}
		}
		cwd, _ := os.Getwd()
		doc := map[string]any{"schema_version": "1", "command": "stub",
			"data": map[string]any{"argv": os.Args[1:], "env": env, "cwd": cwd}}
		b, _ := json.Marshal(doc)
		fmt.Println(string(b))
	}
	os.Exit(exit)
}
