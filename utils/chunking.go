package utils

import "strings"

type Chunk struct {
	Content  string
	Metadata map[string]interface{}
}

func ChunkDocument(content string, chunkSize int, overlap int) []Chunk {
	sentences := strings.Split(content, ". ")
	var chunks []Chunk

	for i := 0; i < len(sentences); i += chunkSize - overlap {
		end := min(i+chunkSize, len(sentences))
		chunk := strings.Join(sentences[i:end], ". ")
		chunks = append(chunks, Chunk{
			Content: chunk,
			Metadata: map[string]interface{}{
				"start_idx": i,
				"end_idx":   end,
			},
		})
	}
	return chunks
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
