package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"rag-server/utils"
	"sort"
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

type queryRequest struct {
	Content string `json:"content"`
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

	if err := r.ParseMultipartForm(32 << 20); err != nil {
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

	const (
		sentencesPerChunk = 15
		overlapSentences  = 3
	)

	var docs []schema.Document
	totalChunks := 0

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

		baseMetadata := map[string]any{
			"file_name":    fileHeader.Filename,
			"content_type": contentType,
		}

		// Switch to sentence-based chunking
		chunks := utils.ChunkDocumentWithMetadata(string(content), baseMetadata, sentencesPerChunk, overlapSentences)

		for _, chunk := range chunks {
			doc := schema.Document{
				PageContent: chunk.Content,
				Metadata:    chunk.Metadata,
			}
			docs = append(docs, doc)
		}

		log.Printf("Processed file: %s (%s) - split into %d chunks", fileHeader.Filename, contentType, len(chunks))
		totalChunks += len(chunks)
	}

	log.Printf("Attempting to add %d document chunks to Weaviate", len(docs))
	_, err := rs.WvClient.AddDocuments(rs.Ctx, docs)
	if err != nil {
		log.Printf("Error adding documents to Weaviate: %v", err)
		http.Error(w, fmt.Sprintf("Failed to add documents: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Successfully added %d document chunks to Weaviate", len(docs))
	utils.RenderJSON(w, map[string]interface{}{
		"message":     fmt.Sprintf("Successfully uploaded %d files (split into %d chunks)", len(files), totalChunks),
		"file_count":  len(files),
		"chunk_count": totalChunks,
	})
}

func (rs *RagServer) QueryHandler(w http.ResponseWriter, r *http.Request) {
	qr := &queryRequest{}
	err := utils.ReadRequestJSON(r, qr)
	log.Printf("Reading request JSON %v", qr)
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
	log.Printf("EnhancedQueryHandler: Received %s request from %s", r.Method, r.RemoteAddr)
	log.Printf("EnhancedQueryHandler: URL path: %s", r.URL.Path)
	log.Printf("EnhancedQueryHandler: Headers: %v", r.Header)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("X-Accel-Buffering", "no")

	if r.Method != http.MethodPost {
		log.Printf("EnhancedQueryHandler: Invalid method: %s", r.Method)
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		Content string `json:"content"`
		Query   string `json:"query"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		log.Printf("EnhancedQueryHandler: Error decoding request body: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Try both content and query fields
	queryText := request.Content
	if queryText == "" {
		queryText = request.Query
	}

	log.Printf("EnhancedQueryHandler: Received query: %s", queryText)

	flusher, ok := w.(http.Flusher)
	if !ok {
		log.Println("EnhancedQueryHandler: Streaming not supported")
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	log.Println("EnhancedQueryHandler: Performing similarity search")
	// Increase the number of results to get more chunks
	docs, err := rs.WvClient.SimilaritySearch(rs.Ctx, queryText, 5) // Increased from 3
	if err != nil {
		log.Printf("EnhancedQueryHandler: Similarity search error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("EnhancedQueryHandler: Found %d document chunks", len(docs))

	// Group chunks by filename for better presentation
	fileChunks := make(map[string][]schema.Document)
	for _, doc := range docs {
		filename := "Unknown source"
		if name, ok := doc.Metadata["file_name"].(string); ok && name != "" {
			filename = name
		}
		fileChunks[filename] = append(fileChunks[filename], doc)
	}

	// Send context information by file
	for filename, chunks := range fileChunks {
		// Sort chunks by index if they're from the same document
		// This uses the chunk_index metadata we added during chunking
		sort.SliceStable(chunks, func(i, j int) bool {
			iIdx, iOk := chunks[i].Metadata["chunk_index"].(int)
			jIdx, jOk := chunks[j].Metadata["chunk_index"].(int)

			if iOk && jOk {
				return iIdx < jIdx
			}
			return false
		})

		log.Printf("EnhancedQueryHandler: Sending %d chunks from %s", len(chunks), filename)

		// Combine chunks for this file into one context message
		var combinedContent strings.Builder
		combinedContent.WriteString(fmt.Sprintf("From %s:\n", filename))

		for _, chunk := range chunks {
			// If it's a multi-chunk document, add chunk info
			if chunkInfo, ok := chunk.Metadata["chunk_info"].(string); ok && chunkInfo != "" {
				combinedContent.WriteString(fmt.Sprintf("\n--- %s ---\n", chunkInfo))
			}
			combinedContent.WriteString(chunk.PageContent)
			combinedContent.WriteString("\n")
		}

		// Format and send the combined context
		contextResp := StreamResponse{
			Type:    "context",
			Content: combinedContent.String(),
		}

		jsonData, err := json.Marshal(contextResp)
		if err != nil {
			log.Printf("EnhancedQueryHandler: Error marshaling context: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		log.Printf("EnhancedQueryHandler: Writing combined context as SSE")
		if _, err := fmt.Fprintf(w, "data: %s\n\n", jsonData); err != nil {
			log.Printf("EnhancedQueryHandler: Error writing context: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		flusher.Flush()
		log.Println("EnhancedQueryHandler: Combined context flushed to client")
	}

	// Prepare LLM query using the same chunks
	var docsContents []string
	for _, doc := range docs {
		filename := "Unknown source"
		if name, ok := doc.Metadata["file_name"].(string); ok && name != "" {
			filename = name
		}

		chunkInfo := ""
		if info, ok := doc.Metadata["chunk_info"].(string); ok {
			chunkInfo = info
		}

		sourceInfo := fmt.Sprintf("From %s%s:\n", filename, chunkInfo)
		docsContents = append(docsContents, sourceInfo+doc.PageContent)
	}

	ragQuery := fmt.Sprintf(ragTemplateStr, queryText, strings.Join(docsContents, "\n"))
	log.Println("EnhancedQueryHandler: Sending query to LLM")

	response, err := llms.GenerateFromSinglePrompt(
		rs.Ctx,
		rs.GeminiClient,
		ragQuery,
		llms.WithModel(rs.ModelName),
	)
	if err != nil {
		log.Printf("EnhancedQueryHandler: LLM error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Println("EnhancedQueryHandler: LLM response received")

	answerResp := StreamResponse{
		Type:    "answer",
		Content: response,
	}

	jsonData, err := json.Marshal(answerResp)
	if err != nil {
		log.Printf("EnhancedQueryHandler: Error marshaling answer: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Format as proper SSE data with "data:" prefix and double newline
	log.Printf("EnhancedQueryHandler: Writing answer as SSE")
	if _, err := fmt.Fprintf(w, "data: %s\n\n", jsonData); err != nil {
		log.Printf("EnhancedQueryHandler: Error writing answer: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	flusher.Flush()
	log.Println("EnhancedQueryHandler: Answer flushed to client")
}
