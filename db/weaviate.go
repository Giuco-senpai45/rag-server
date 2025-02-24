package db

import (
	"cmp"
	"context"
	"log"
	"os"

	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/vectorstores/weaviate"
)

func InitWeaviate(ctx context.Context, emb *embeddings.EmbedderImpl) (weaviate.Store, error) {
	addr := "weaviate-db:" + cmp.Or(os.Getenv("WEAVIATE_PORT"), "9035")
	log.Println("Connecting to Weaviate at" + addr)

	wconn, err := weaviate.New(
		weaviate.WithEmbedder(emb),
		weaviate.WithScheme("http"),
		weaviate.WithHost(addr),
		weaviate.WithIndexName("Documents"),
	)
	if err != nil {
		log.Fatalf("could not create weaviate client: %v", err)
	}
	log.Println(wconn)
	log.Println("Weaviate client created")

	return wconn, nil
}
