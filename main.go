package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"rag-server/db"
	"rag-server/routes"
	"rag-server/server"
	"time"

	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms/googleai"
)

const model_name = "gemini-1.5-flash"

const embedding_model = "text-embedding-004"

func main() {
	ctx := context.Background()

	ak := os.Getenv("GEMINI_API_KEY")
	gc, err := googleai.New(ctx,
		googleai.WithAPIKey(ak),
		googleai.WithDefaultEmbeddingModel(embedding_model),
	)
	log.Println("Google AI client created")

	if err != nil {
		log.Fatal(err)
	}

	emb, err := embeddings.NewEmbedder(gc)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Embedder created")

	wconn, err := db.InitWeaviate(ctx, emb)
	if err != nil {
		log.Fatal(err)
	}

	rs := &server.RagServer{
		Ctx:          ctx,
		WvClient:     wconn,
		GeminiClient: gc,
		ModelName:    model_name,
	}

	mux := http.NewServeMux()
	routes.RegisterRoutes(mux, rs)

	address := fmt.Sprintf(":%s", os.Getenv("PORT"))
	if address == "" {
		address = ":8080"
	}

	server := &http.Server{
		Addr:    address,
		Handler: mux,
	}

	done := make(chan struct{})
	go func() {

		log.Printf("Server started on port %s", address)
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("failed to listen and serve %v", slog.Any("error", err))
		}
		close(done)
	}()

	select {
	case <-done:
		break
	case <-ctx.Done():
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		server.Shutdown(ctx)
		cancel()
	}
}
