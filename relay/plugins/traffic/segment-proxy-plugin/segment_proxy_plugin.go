package segment_proxy_plugin

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/immersa-co/relay-core/relay/config"
	"github.com/immersa-co/relay-core/relay/traffic"
)

var (
	Factory    segmentProxyPluginFactory
	pluginName = "segment-proxy"
	logger     = log.New(os.Stdout, fmt.Sprintf("[traffic-%s] ", pluginName), 0)
)

type segmentProxyPluginFactory struct{}

func (f segmentProxyPluginFactory) Name() string {
	return pluginName
}

func (f segmentProxyPluginFactory) New(configSection *config.Section) (traffic.Plugin, error) {
	return &segmentProxyPlugin{}, nil
}

type segmentProxyPlugin struct{}

func (plug segmentProxyPlugin) Name() string {
	return pluginName
}

// Event represents a single event in the Fullstory data structure
type Event struct {
	Kind int             `json:"Kind"`
	Args json.RawMessage `json:"Args"`
	When int             `json:"When,omitempty"`
}

type SegmentData struct {
	Seq      int     `json:"Seq,omitempty"`
	When     int     `json:"When,omitempty"`
	WriteKey string  `json:"writeKey"`
	Evts     []Event `json:"Evts"`
}

func (plug segmentProxyPlugin) HandleRequest(
	response http.ResponseWriter,
	request *http.Request,
	info traffic.RequestInfo,
) bool {
	if info.Serviced {
		return false
	}
	if request.URL.Path != "/rec/bundle/v2" {
		return false
	}

	if request.Body == nil {
		return false
	}
	originalBodyBytes, err := io.ReadAll(request.Body)
	if err != nil {
		logger.Printf("Failed to read request body: %v", err)
		return false
	}
	request.Body.Close() 

	request.Body = io.NopCloser(bytes.NewReader(originalBodyBytes))

	var contentBytes []byte

	if request.Header.Get("Content-Encoding") == "gzip" {
		bodyReader := bytes.NewReader(originalBodyBytes)
		reader, err := gzip.NewReader(bodyReader)
		if err != nil {
			logger.Printf("Failed to create gzip reader: %v", err)
			return false 
		}
		defer reader.Close()

		contentBytes, err = io.ReadAll(reader)
		if err != nil {
			logger.Printf("Failed to decompress gzip body: %v", err)
			return false 
		}
	} else {
		contentBytes = originalBodyBytes
	}

	var navigateEvent = 37	
	var segmentData SegmentData
	if err := json.Unmarshal(contentBytes, &segmentData); err != nil {
		return false
	} else {
		for _, event := range segmentData.Evts {
			if event.Kind == navigateEvent {
				request.URL.Path = "/v1/page"
				request.Method = "POST"

				var args []string
				if err := json.Unmarshal(event.Args, &args); err != nil {
					return false
				}

				if len(args) == 0 {
					return false
				}

				url := args[0]
				requestBody := map[string]interface{}{
					"writeKey": segmentData.WriteKey,
					"userId":    request.URL.Query().Get("UserId"),
					"timestamp": time.Now().Unix(),
					"properties": map[string]interface{}{
						"url": url,
					},
					"name": "track " + url,
				}

				jsonBody, err := json.Marshal(requestBody)
				if err != nil {
					logger.Printf("Failed to marshal request body: %v", err)
					return false
				}

				request.Body = io.NopCloser(bytes.NewReader(jsonBody))
				
				contentLength := int64(len(jsonBody))
				request.ContentLength = contentLength
				request.Header.Set("Content-Length", fmt.Sprintf("%d", contentLength))
			} 
		}
	}

	return false
} 