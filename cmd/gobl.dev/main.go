// gobl.dev serves the GOBL HTTP API alongside a web editor UI.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/invopop/gobl"
	goblapi "github.com/invopop/gobl.dev/api"
	_ "github.com/invopop/gobl.dev/bundle"
	"github.com/invopop/gobl.dev/editor"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	const defaultPort = "8080"
	var port string
	flag.StringVar(&port, "p", defaultPort, "port to listen on")
	flag.Parse()

	if env := os.Getenv("PORT"); env != "" && port == defaultPort {
		port = env
	}

	handler := goblapi.NewHandler(
		goblapi.WithMCP(),
		goblapi.WithFavicon(),
		goblapi.WithRoutes(func(mux *http.ServeMux, _ string) {
			editor.RegisterAssets(mux)
			mux.HandleFunc("GET /{$}", goblapi.WithETag(editor.Handler()))
		}),
	)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: handler,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Printf("GOBL %s — listening on :%s", gobl.VERSION, port)

	var startErr error
	go func() {
		err := srv.ListenAndServe()
		if !errors.Is(err, http.ErrServerClosed) {
			startErr = err
		}
		stop()
	}()

	<-ctx.Done()
	log.Println("Shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	return startErr
}
