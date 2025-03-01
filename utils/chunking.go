package utils

import (
	"fmt"
	"strings"
)

type Chunk struct {
	Content  string
	Metadata map[string]interface{}
}

func ChunkDocumentWithMetadata(content string, originalMetadata map[string]any, chunkSize int, overlap int) []Chunk {
	// Split by sentences for more natural chunks
	sentences := strings.Split(content, ". ")
	var chunks []Chunk
	totalChunks := (len(sentences) + chunkSize - overlap - 1) / (chunkSize - overlap)
	if totalChunks < 1 {
		totalChunks = 1
	}

	for i := 0; i < len(sentences); i += chunkSize - overlap {
		end := min(i+chunkSize, len(sentences))
		chunk := strings.Join(sentences[i:end], ". ")

		metadata := make(map[string]interface{})
		for k, v := range originalMetadata {
			metadata[k] = v
		}

		chunkNum := (i / (chunkSize - overlap)) + 1
		metadata["chunk_index"] = chunkNum - 1
		metadata["chunk_count"] = totalChunks

		if totalChunks > 1 {
			metadata["chunk_info"] = fmt.Sprintf(" (part %d/%d)", chunkNum, totalChunks)
		} else {
			metadata["chunk_info"] = ""
		}

		chunks = append(chunks, Chunk{
			Content:  chunk,
			Metadata: metadata,
		})
	}
	return chunks
}

// Alternative chunking method that works with characters instead of sentences
func ChunkDocumentByChars(content string, originalMetadata map[string]any, chunkSize int, overlap int) []Chunk {
	if len(content) <= chunkSize {
		metadata := make(map[string]interface{})
		for k, v := range originalMetadata {
			metadata[k] = v
		}
		metadata["chunk_index"] = 0
		metadata["chunk_count"] = 1
		metadata["chunk_info"] = ""

		return []Chunk{{
			Content:  content,
			Metadata: metadata,
		}}
	}

	contentRunes := []rune(content)
	contentLength := len(contentRunes)

	var chunks []Chunk
	totalChunks := (contentLength + chunkSize - overlap - 1) / (chunkSize - overlap)
	if totalChunks < 1 {
		totalChunks = 1
	}

	for i := 0; i < contentLength; i += (chunkSize - overlap) {
		end := i + chunkSize
		if end > contentLength {
			end = contentLength
		}

		metadata := make(map[string]interface{})
		for k, v := range originalMetadata {
			metadata[k] = v
		}

		chunkNum := (i / (chunkSize - overlap)) + 1
		metadata["chunk_index"] = chunkNum - 1
		metadata["chunk_count"] = totalChunks

		if totalChunks > 1 {
			metadata["chunk_info"] = fmt.Sprintf(" (part %d/%d)", chunkNum, totalChunks)
		} else {
			metadata["chunk_info"] = ""
		}

		chunk := string(contentRunes[i:end])
		chunks = append(chunks, Chunk{
			Content:  chunk,
			Metadata: metadata,
		})

		if end == contentLength {
			break
		}
	}

	return chunks
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
