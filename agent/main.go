package main

import (
	"fmt"
	"strings"
)

type LogEvent struct {
	Timestamp string
	Level     string
	Message   string
}

func main() {
	line := "2026-07-22 ERROR pod crashed: out of memory"

	parts := strings.Split(line, " ")

	event := LogEvent{
		Timestamp: parts[0],
		Level:     parts[1],
		Message:   strings.Join(parts[2:], " "),
	}
	fmt.Println("Timestamp:", event.Timestamp)
	fmt.Println("Level:", event.Level)
	fmt.Println("Message:", event.Message)
}
