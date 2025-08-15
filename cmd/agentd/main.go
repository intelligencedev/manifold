package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprintln(w, "ok"); err != nil {
			log.Printf("failed to write health response: %v", err)
		}
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprintln(w, "ready"); err != nil {
			log.Printf("failed to write ready response: %v", err)
		}
	})
	log.Println("agentd listening on :32180")
	if err := http.ListenAndServe(":32180", mux); err != nil {
		log.Fatal(err)
	}
}
