package content_enricher_plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/fullstorydev/relay-core/relay/config"
	"github.com/fullstorydev/relay-core/relay/traffic"
	"github.com/fullstorydev/relay-core/relay/version"
)

var (
	Factory    contentEnricherPluginFactory
	pluginName = "enrich-content"
	logger     = log.New(os.Stdout, fmt.Sprintf("[traffic-%s] ", pluginName), 0)

	PluginVersionHeaderName = "X-Relay-Content-Enricher-Version"
)

type configStructure struct {
	Body    map[string]interface{} `yaml:"body,omitempty"`
	Headers map[string]string      `yaml:"headers,omitempty"`
}

type contentEnricherPluginFactory struct{}

func (f contentEnricherPluginFactory) Name() string {
	return pluginName
}

func (f contentEnricherPluginFactory) New(configSection *config.Section) (traffic.Plugin, error) {
	plugin := &contentEnricherPlugin{
		bodyEnrichments:   make(map[string]interface{}),
		headerEnrichments: make(map[string]string),
	}

	if err := config.ParseOptional(configSection, "body", func(_ string, value map[string]interface{}) error {
		for k, v := range value {
			plugin.bodyEnrichments[k] = v
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("error parsing body enrichments: %v", err)
	}

	if err := config.ParseOptional(configSection, "headers", func(_ string, value map[string]string) error {
		for k, v := range value {
			plugin.headerEnrichments[k] = v
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("error parsing header enrichments: %v", err)
	}

	if len(plugin.bodyEnrichments) == 0 && len(plugin.headerEnrichments) == 0 {
		logger.Println("No enrichments configured, plugin will not be loaded.")
		return nil, nil
	}

	logger.Printf("Initialized with %d body enrichments and %d header enrichments", len(plugin.bodyEnrichments), len(plugin.headerEnrichments))
	return plugin, nil
}

type contentEnricherPlugin struct {
	bodyEnrichments   map[string]interface{}
	headerEnrichments map[string]string
}

func (plug *contentEnricherPlugin) Name() string {
	return pluginName
}

func (plug *contentEnricherPlugin) HandleRequest(
	response http.ResponseWriter,
	request *http.Request,
	info traffic.RequestInfo,
) bool {
	if info.Serviced {
		return false
	}

	if serviced := plug.enrichHeaderContent(response, request); serviced {
		return true
	}
	if serviced := plug.enrichBodyContent(response, request); serviced {
		return true
	}

	if len(plug.headerEnrichments) > 0 || len(plug.bodyEnrichments) > 0 {
		request.Header.Add(PluginVersionHeaderName, version.RelayRelease)
	}

	return false
}

func (plug *contentEnricherPlugin) enrichHeaderContent(response http.ResponseWriter, request *http.Request) bool {
	if len(plug.headerEnrichments) == 0 {
		return false
	}

	for header, value := range plug.headerEnrichments {
		request.Header.Set(header, value)
	}
	logger.Printf("Enriched headers: %v", plug.headerEnrichments)

	return false
}

func (plug *contentEnricherPlugin) enrichBodyContent(response http.ResponseWriter, request *http.Request) bool {
	if len(plug.bodyEnrichments) == 0 {
		return false
	}

	contentType := request.Header.Get("Content-Type")
	if contentType != "application/json" {
		logger.Printf("Skipping body enrichment for non-JSON content type: %s", contentType)
		return false
	}

	if request.Body == nil || request.Body == http.NoBody {
		logger.Println("Skipping body enrichment for empty body")
		return false
	}

	bodyBytes, err := io.ReadAll(request.Body)
	request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	if err != nil {
		logger.Printf("Error reading request body: %s", err)
		http.Error(response, fmt.Sprintf("Error reading request body: %s", err), http.StatusInternalServerError)
		return true
	}

	if len(bodyBytes) == 0 {
		logger.Println("Skipping body enrichment for zero-length body after read")
		return false
	}

	var jsonBody map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &jsonBody); err != nil {
		logger.Printf("Error parsing JSON body, cannot enrich: %s. Body: %s", err, string(bodyBytes))
		request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		return false
	}

	for key, value := range plug.bodyEnrichments {
		if _, exists := jsonBody[key]; !exists {
			jsonBody[key] = value
		} else {
			logger.Printf("Skipping enrichment for body key '%s' because it already exists.", key)
		}
	}

	enrichedBodyBytes, err := json.Marshal(jsonBody)
	if err != nil {
		logger.Printf("Error marshaling enriched JSON: %s", err)
		http.Error(response, fmt.Sprintf("Error marshaling enriched JSON: %s", err), http.StatusInternalServerError)
		return true
	}

	request.Body = io.NopCloser(bytes.NewBuffer(enrichedBodyBytes))
	request.ContentLength = int64(len(enrichedBodyBytes))
	request.Header.Set("Content-Length", fmt.Sprintf("%d", request.ContentLength))

	logger.Printf("Enriched body. New length: %d", request.ContentLength)

	return false
}

/*
Copyright 2024 Immersa

Permission is hereby granted, free of charge, to any person obtaining a copy of this software
and associated documentation files (the "Software"), to deal in the Software without restriction,
including without limitation the rights to use, copy, modify, merge, publish, distribute,
sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or
substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT
NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY,
WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/
