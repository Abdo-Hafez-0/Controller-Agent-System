package main

import (
	"fmt"
	"io"
	"net/http"
)

func commandHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	command := string(body)
	fmt.Println("Received: ", command)

	var response string

	switch command {
	case "lock":
		response = "Device locked"

	case "shutdown":
		response = "Device shutting down"

	case "wallpaper":
		response = "Wallpaper changed"

	default:
		response = "Unknown command"
	}
	w.Write([]byte(response))
}

func main() {

	server := http.Server{
		Addr:"http://127.0.0.1:1010",
	}
	http.HandleFunc("/command", commandHandler)

	fmt.Println("Agent running on :1010")
	server.ListenAndServe()
}