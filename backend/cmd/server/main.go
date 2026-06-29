package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"postman-transform/backend-golang/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx); err != nil && err != context.Canceled {
		log.Printf("server stopped: %v", err)
		os.Exit(1)
	}
	_ = os.Stdout
}
