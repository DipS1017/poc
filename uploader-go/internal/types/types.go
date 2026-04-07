package types

// InitiateRequest - POST /api/v1/initiate
type InitiateRequest struct {
	FileName    string `json:"fileName"`
	FileSize    int64  `json:"fileSize"`
	ContentType string `json:"contentType"`
}

type InitiateResponse struct {
	UploadID   string `json:"uploadId"`
	Key        string `json:"key"`
	PartSize   int64  `json:"partSize"`
	TotalParts int    `json:"totalParts"`
}

// SignPartRequest - POST /api/v1/sign
type SignPartRequest struct {
	UploadID   string `json:"uploadId"`
	Key        string `json:"key"`
	PartNumber int    `json:"partNumber"`
}

type SignPartResponse struct {
	URL string `json:"url"`
}

// ListPartsResponse - GET /api/v1/uploads/:id/parts
type PartInfo struct {
	PartNumber int    `json:"partNumber"`
	ETag       string `json:"etag"`
	Size       int64  `json:"size"`
}

type ListPartsResponse struct {
	Parts []PartInfo `json:"parts"`
}

// CompleteRequest - POST /api/v1/complete
type CompletedPart struct {
	PartNumber int    `json:"partNumber"`
	ETag       string `json:"etag"`
}

type CompleteRequest struct {
	UploadID string          `json:"uploadId"`
	Key      string          `json:"key"`
	Parts    []CompletedPart `json:"parts"`
}

type CompleteResponse struct {
	Location string `json:"location"`
	Key      string `json:"key"`
}

// AbortRequest - DELETE /api/v1/uploads/:id
// Uses URL param :id for uploadId and query param key

// ErrorResponse for API errors
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details string `json:"details,omitempty"`
}
