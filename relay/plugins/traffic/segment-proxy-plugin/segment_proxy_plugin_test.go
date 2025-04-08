package segment_proxy_plugin

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/immersa-co/relay-core/relay/traffic"
)

func TestSegmentProxyPlugin(t *testing.T) {
	plugin := &segmentProxyPlugin{}

	tests := []struct {
		name           string
		path           string
		query          string
		body           []byte
		expectedPath   string
		expectedMethod string
		expectedBody   map[string]interface{}
		shouldHandle   bool
	}{
		{
			name:  "navigate event should be transformed",
			path:  "/rec/bundle/v2",
			query: "writeKey=test-key&UserId=test-user",
			body: func() []byte {
				data := SegmentData{
					WriteKey: "test-key",
					Evts: []Event{
						{
							Kind: 37,
							Args: json.RawMessage(`["https://example.com"]`),
						},
					},
				}
				bytes, _ := json.Marshal(data)
				return bytes
			}(),
			expectedPath:   "/v1/page",
			expectedMethod: "POST",
			expectedBody: map[string]interface{}{
				"writeKey":   "test-key",
				"userId":     "test-user",
				"properties": map[string]interface{}{
					"url": "https://example.com",
				},
				"name": "track https://example.com",
			},
			shouldHandle: false,
		},
		{
			name:  "non-navigate event should not be transformed",
			path:  "/rec/bundle/v2",
			query: "writeKey=test-key&UserId=test-user",
			body: func() []byte {
				data := SegmentData{
					Evts: []Event{
						{
							Kind: 1,
							Args: json.RawMessage(`["other-event"]`),
						},
					},
				}
				bytes, _ := json.Marshal(data)
				return bytes
			}(),
			expectedPath:   "/rec/bundle/v2",
			expectedMethod: "GET",
			shouldHandle:   false,
		},
		{
			name:           "non-matching path should not be transformed",
			path:           "/other/path",
			query:          "writeKey=test-key&UserId=test-user",
			body:           []byte(`{}`),
			expectedPath:   "/other/path",
			expectedMethod: "GET",
			shouldHandle:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path+"?"+tt.query, bytes.NewReader(tt.body))
			w := httptest.NewRecorder()

			handled := plugin.HandleRequest(w, req, traffic.RequestInfo{})

			if handled != tt.shouldHandle {
				t.Errorf("HandleRequest() returned %v, want %v", handled, tt.shouldHandle)
			}

			if req.URL.Path != tt.expectedPath {
				t.Errorf("Path = %v, want %v", req.URL.Path, tt.expectedPath)
			}

			if req.Method != tt.expectedMethod {
				t.Errorf("Method = %v, want %v", req.Method, tt.expectedMethod)
			}

			if tt.expectedBody != nil {
				var actualBody map[string]interface{}
				if err := json.NewDecoder(req.Body).Decode(&actualBody); err != nil {
					t.Fatalf("Failed to decode request body: %v", err)
				}

				// Skip timestamp comparison as it's dynamic
				delete(actualBody, "timestamp")

				if !reflect.DeepEqual(actualBody, tt.expectedBody) {
					t.Errorf("Body = %v, want %v", actualBody, tt.expectedBody)
				}
			}
		})
	}
} 