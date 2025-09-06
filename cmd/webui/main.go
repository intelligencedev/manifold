package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"singularityio/internal/webui"
)

func main() {
	host := os.Getenv("WEB_UI_HOST")
	if host == "" {
		host = "0.0.0.0"
	}
	port := os.Getenv("WEB_UI_PORT")
	if port == "" {
		port = "8081"
	}
	addr := host + ":" + port

	mux := http.NewServeMux()
	webui.Register(mux)

	srv := &http.Server{Addr: addr, Handler: mux}

	go func() {
		log.Printf("webui listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	// graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	} else {
		log.Printf("webui stopped")
	}
}
