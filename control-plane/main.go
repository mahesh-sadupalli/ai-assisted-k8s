// Command control-plane is Sentinel's self-service deploy API.
package main

import (
	"log/slog"
	"net/http"
	"os"
	"strings"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	guardrail := Guardrail{
		// comma-separated image prefixes, e.g. "ghcr.io/me/,123.dkr.ecr.eu-central-1.amazonaws.com/"
		AllowedRegistries: strings.Split(getenv("ALLOWED_REGISTRIES", "ghcr.io/mahesh-sadupalli/"), ","),
		DefaultQuota:      Quota{CPUMillis: 2000, MemoryMiB: 2048},
	}

	addr := getenv("ADDR", ":8080")
	srv := NewServer(NewStore(), guardrail, log)
	log.Info("control-plane listening", "addr", addr, "allowed_registries", guardrail.AllowedRegistries)
	if err := http.ListenAndServe(addr, srv.Routes()); err != nil {
		log.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
