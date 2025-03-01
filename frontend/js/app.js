const ragApp = () => {
  return {
    files: [],
    query: "",
    enhancedQuery: "",
    response: "",
    enhancedResponse: "",
    rawEnhancedResponse: "",
    contexts: [],
    formattedContexts: [],
    uploading: false,
    querying: false,
    enhancedQuerying: false,
    uploadSuccess: false,
    uploadError: "",

    init() {
      marked.setOptions({
        breaks: true,
        gfm: true,
        highlight: function(code, lang) {
          if (lang && hljs.getLanguage(lang)) {
            return hljs.highlight(code, { language: lang }).value;
          }
          return hljs.highlightAuto(code).value;
        }
      });
    },

    formatMarkdown(text) {
      if (!text) return '';
      return marked.parse(text);
    },

    async uploadFiles() {
      this.uploading = true;
      this.uploadSuccess = false;
      this.uploadError = "";

      const formData = new FormData();
      for (let file of this.files) {
        formData.append("documents", file);
      }

      try {
        const response = await fetch("/api/context", {
          method: "POST",
          body: formData,
        });

        if (!response.ok) {
          throw new Error(`Upload failed: ${response.statusText}`);
        }

        const result = await response.json();
        this.uploadSuccess = true;
        this.files = [];
      } catch (error) {
        console.error("Upload error:", error);
        this.uploadError = error.message;
      } finally {
        this.uploading = false;
      }
    },

    async queryDocuments() {
      if (!this.query) return;

      this.querying = true;
      try {
        const response = await fetch("/api/query", {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify({
            content: this.query,
          }),
        });

        if (response.ok) {
          this.response = await response.text();
        }
      } catch (error) {
        console.error("Query error:", error);
      } finally {
        this.querying = false;
      }
    },

    async enhancedQueryDocuments() {
      if (!this.enhancedQuery) return;
    
      this.enhancedQuerying = true;
      this.contexts = [];
      this.formattedContexts = [];
      this.rawEnhancedResponse = "";
      this.enhancedResponse = "";
    
      try {
        console.log("Sending request to /api/enhanced-query");
        const response = await fetch("/api/enhanced-query", {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify({
            content: this.enhancedQuery,
          }),
        });
    
        if (!response.ok) {
          throw new Error(`Query failed: ${response.statusText}`);
        }
    
        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        console.log("Starting to read SSE stream");
    
        while (true) {
          const { done, value } = await reader.read();
          if (done) {
            console.log("Stream ended");
            break;
          }
    
          const text = decoder.decode(value);
          console.log("Received chunk:", text);
          
          const lines = text.split("\n");
    
          for (const line of lines) {
            if (!line.trim() || !line.startsWith("data:")) continue;
    
            try {
              const jsonStr = line.substring(5).trim();
              console.log("Extracted JSON string:", jsonStr);
              
              const data = JSON.parse(jsonStr);
              console.log("Parsed data:", data);
              
              if (data.type === "context") {
                this.contexts.push(data.content);
                this.formattedContexts.push(this.formatMarkdown(data.content));
              } else if (data.type === "answer") {
                // Store raw response
                this.rawEnhancedResponse = data.content;
                this.enhancedResponse = this.formatMarkdown(data.content);
              }
            } catch (error) {
              console.error("Error parsing SSE data:", error, "Line:", line);
            }
          }
        }
      } catch (error) {
        console.error("Enhanced query error:", error);
        this.rawEnhancedResponse = "Error: " + error.message;
        this.enhancedResponse = this.rawEnhancedResponse;
      } finally {
        this.enhancedQuerying = false;
      }
    },
  };
};
