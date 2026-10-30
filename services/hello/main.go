// Command hello is the sample service Sentinel deploys.
//
// Set LEAK_MIB_PER_SEC to make it allocate memory until the container is OOMKilled,
// which gives the agent a real failure to diagnose.
package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"
)

func leak(mibPerSec int) {
	var hoard [][]byte
	for range time.Tick(time.Second) {
		b := make([]byte, mibPerSec<<20)
		for i := range b {
			b[i] = 1 // touch pages so they count against the limit
		}
		hoard = append(hoard, b)
		slog.Warn("cache grew", "held_mib", len(hoard)*mibPerSec)
	}
}

func main() {
	if n, _ := strconv.Atoi(os.Getenv("LEAK_MIB_PER_SEC")); n > 0 {
		go leak(n)
	}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "hello from", os.Getenv("HOSTNAME"))
	})
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	slog.Info("hello listening", "addr", ":8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
