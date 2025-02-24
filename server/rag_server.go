package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"rag-server/utils"
	"strings"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/googleai"
	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/vectorstores/weaviate"
)

type RagServer struct {
	Ctx          context.Context
	WvClient     weaviate.Store
	GeminiClient *googleai.GoogleAI
	ModelName    string
}

type document struct {
	Text        string
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type"` // "text/markdown" or "text/plain"
}

type addRequest struct {
	Files []*multipart.FileHeader `form:"files"`
}

type queryRequest struct {
	Content string
}

const ragTemplateStr = `
I will ask you a question and will provide some additional context information.
Assume this context information is factual and correct, as part of internal
documentation.
If the question relates to the context, answer it using the context.
If the question does not relate to the context, answer it as normal.

For example, let's say the context has nothing in it about tropical flowers;
then if I ask you about tropical flowers, just answer what you know about them
without referring to the context.

For example, if the context does mention minerology and I ask you about that,
provide information from the context along with general knowledge.

Question:
%s

Context:
%s
`

func (rs *RagServer) AddDocumentHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received document upload request")

	if err := r.ParseMultipartForm(32 << 20); err != nil { // 32MB max memory
		log.Printf("Error parsing multipart form: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	files := r.MultipartForm.File["documents"]
	if len(files) == 0 {
		log.Printf("No files received")
		http.Error(w, "no files uploaded", http.StatusBadRequest)
		return
	}

	var docs []schema.Document

	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			log.Printf("Error opening file %s: %v", fileHeader.Filename, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer file.Close()

		content, err := io.ReadAll(file)
		if err != nil {
			log.Printf("Error reading file %s: %v", fileHeader.Filename, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		contentType := "text/plain"
		if strings.HasSuffix(strings.ToLower(fileHeader.Filename), ".md") {
			contentType = "text/markdown"
		}

		doc := schema.Document{
			PageContent: string(content),
			Metadata: map[string]any{
				"file_name":    fileHeader.Filename,
				"content_type": contentType,
			},
		}
		docs = append(docs, doc)
		log.Printf("Processed file: %s (%s)", fileHeader.Filename, contentType)
	}

	log.Printf("Attempting to add %d documents to Weaviate", len(docs))
	_, err := rs.WvClient.AddDocuments(rs.Ctx, docs)
	if err != nil {
		log.Printf("Error adding documents to Weaviate: %v", err)
		http.Error(w, fmt.Sprintf("Failed to add documents: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Successfully added %d documents to Weaviate", len(docs))
	utils.RenderJSON(w, map[string]string{
		"message": fmt.Sprintf("Successfully uploaded %d files", len(docs)),
	})
}

func (rs *RagServer) QueryHandler(w http.ResponseWriter, r *http.Request) {
	qr := &queryRequest{}
	err := utils.ReadRequestJSON(r, qr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Println("Query received")

	docs, err := rs.WvClient.SimilaritySearch(rs.Ctx, qr.Content, 3)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Println("Documents retrieved")

	var docsContents []string
	for _, doc := range docs {
		sourceInfo := fmt.Sprintf("From %s:\n", doc.Metadata["file_name"])
		docsContents = append(docsContents, sourceInfo+doc.PageContent)
	}
	log.Println("Documents contents retrieved")

	// create query for LLM with the most relevant documents as context
	ragQuery := fmt.Sprintf(ragTemplateStr, qr.Content, strings.Join(docsContents, "\n"))
	respText, err := llms.GenerateFromSinglePrompt(rs.Ctx, rs.GeminiClient, ragQuery, llms.WithModel(rs.ModelName))
	if err != nil {
		log.Printf("calling generative model: %v", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Println("Response generated")

	utils.RenderJSON(w, respText)
}

type StreamResponse struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

func (rs *RagServer) EnhancedQueryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		Query string `json:"query"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	docs, err := rs.WvClient.SimilaritySearch(rs.Ctx, request.Query, 3)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for _, doc := range docs {
		contextResp := StreamResponse{
			Type:    "context",
			Content: fmt.Sprintf("From %s: %s", doc.Metadata["file_name"], doc.PageContent),
		}
		if err := json.NewEncoder(w).Encode(contextResp); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "\n")
		flusher.Flush()
	}

	var docsContents []string
	for _, doc := range docs {
		sourceInfo := fmt.Sprintf("From %s:\n", doc.Metadata["file_name"])
		docsContents = append(docsContents, sourceInfo+doc.PageContent)
	}
	ragQuery := fmt.Sprintf(ragTemplateStr, request.Query, strings.Join(docsContents, "\n"))

	response, err := llms.GenerateFromSinglePrompt(
		rs.Ctx,
		rs.GeminiClient,
		ragQuery,
		llms.WithModel(rs.ModelName),
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	answerResp := StreamResponse{
		Type:    "answer",
		Content: response,
	}
	if err := json.NewEncoder(w).Encode(answerResp); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "\n")
	flusher.Flush()
}
