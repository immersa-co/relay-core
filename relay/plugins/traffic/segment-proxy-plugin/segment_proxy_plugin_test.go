package segment_proxy_plugin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/immersa-co/relay-core/relay/traffic"
)

func TestSegmentProxyPlugin(t *testing.T) {
	// Create a test HTTP server to mock target endpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create plugin with mocked HTTP client
	plugin := &segmentProxyPlugin{
		client: server.Client(),
	}

	tests := []struct {
		name              string
		path              string
		query             string
		body              []byte
		expectedStatus    int
		shouldService     bool
		expectedEventCount int
	}{
		{
			name:              "single navigate event should be processed",
			path:              "/rec/bundle/v2",
			query:             "writeKey=test-key&UserId=test-user",
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
			expectedStatus:    http.StatusOK,
			shouldService:     false, // Always return false to avoid "serviced" log
			expectedEventCount: 1,
		},
		{
			name:              "multiple navigate events should be processed",
			path:              "/rec/bundle/v2",
			query:             "writeKey=test-key&UserId=test-user",
			body: func() []byte {
				data := SegmentData{
					WriteKey: "test-key",
					Evts: []Event{
						{
							Kind: 37,
							Args: json.RawMessage(`["https://example.com"]`),
						},
						{
							Kind: 1,
							Args: json.RawMessage(`["not-a-navigate-event"]`),
						},
						{
							Kind: 37,
							Args: json.RawMessage(`["https://example.org"]`),
						},
					},
				}
				bytes, _ := json.Marshal(data)
				return bytes
			}(),
			expectedStatus:    http.StatusOK,
			shouldService:     false,
			expectedEventCount: 2,
		},
		{
			name:              "path containing rec/bundle/v2 should be processed",
			path:              "/api/v1/rec/bundle/v2/data",
			query:             "writeKey=test-key&UserId=test-user",
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
			expectedStatus:    http.StatusOK,
			shouldService:     false,
			expectedEventCount: 1,
		},
		{
			name:              "non-navigate event should not be processed",
			path:              "/rec/bundle/v2",
			query:             "writeKey=test-key&UserId=test-user",
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
			expectedStatus:    0, // No response status set
			shouldService:     false,
			expectedEventCount: 0,
		},
		{
			name:              "non-matching path should not be processed",
			path:              "/other/path",
			query:             "writeKey=test-key&UserId=test-user",
			body:              []byte(`{}`),
			expectedStatus:    0,
			shouldService:     false,
			expectedEventCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test request with the test parameters
			req := httptest.NewRequest("GET", "http://example.com"+tt.path+"?"+tt.query, bytes.NewReader(tt.body))
			w := httptest.NewRecorder()

			// Create a counter to track HTTP requests made by the plugin
			requestsMade := 0
			originalTransport := server.Client().Transport
			server.Client().Transport = &countingTransport{
				transport: originalTransport,
				callback: func() {
					requestsMade++
				},
			}

			// Call the plugin handler
			handled := plugin.HandleRequest(w, req, traffic.RequestInfo{})

			// Check if the handler returned the expected servicing value
			if handled != tt.shouldService {
				t.Errorf("HandleRequest() returned %v, want %v", handled, tt.shouldService)
			}

			// Check if the correct number of requests were made to the target
			if requestsMade != tt.expectedEventCount {
				t.Errorf("Expected %d requests to be made, but got %d", tt.expectedEventCount, requestsMade)
			}

			// Check if the response status is as expected
			if tt.expectedStatus != 0 && w.Code != tt.expectedStatus {
				t.Errorf("Expected status code %d, but got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

// countingTransport is an http.RoundTripper that counts requests
type countingTransport struct {
	transport http.RoundTripper
	callback  func()
}

func (t *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Call the callback to track the request
	t.callback()
	// Forward to the underlying transport
	return t.transport.RoundTrip(req)
} 