const ragApp = () => {
    return {
        files: [],
        query: '',
        response: '',
        uploading: false,
        querying: false,
        uploadSuccess: false,
        uploadError: '',
        
        async uploadFiles() {
            this.uploading = true;
            this.uploadSuccess = false;
            this.uploadError = '';
            
            const formData = new FormData();
            for (let file of this.files) {
                formData.append('files', file);
            }
            
            try {
                const response = await fetch('/api/add', {
                    method: 'POST',
                    body: formData
                });
                
                if (!response.ok) {
                    throw new Error(`Upload failed: ${response.statusText}`);
                }
                
                const result = await response.json();
                this.uploadSuccess = true;
                this.files = [];
            } catch (error) {
                console.error('Upload error:', error);
                this.uploadError = error.message;
            } finally {
                this.uploading = false;
            }
        },
        
        async queryDocuments() {
            if (!this.query) return
            
            this.querying = true
            try {
                const response = await fetch('http://localhost:8080/query', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({
                        content: this.query
                    })
                })
                
                if (response.ok) {
                    this.response = await response.text()
                }
            } catch (error) {
                console.error('Query error:', error)
            } finally {
                this.querying = false
            }
        }
    }
}