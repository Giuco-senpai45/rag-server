package routes

import (
	"net/http"
	"rag-server/server"
)

func RegisterRoutes(router *http.ServeMux, server *server.RagServer) {
	router.HandleFunc("POST /context", enableCORS(server.AddDocumentHandler))
	router.HandleFunc("POST /query", enableCORS(server.QueryHandler))
	router.HandleFunc("POST /enhanced-query", enableCORS(server.EnhancedQueryHandler))

	router.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func enableCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, r)
	}
}
