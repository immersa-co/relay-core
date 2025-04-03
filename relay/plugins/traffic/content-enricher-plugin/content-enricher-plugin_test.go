package content_enricher_plugin_test

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/fullstorydev/relay-core/catcher"
	"github.com/fullstorydev/relay-core/relay"
	content_enricher_plugin "github.com/fullstorydev/relay-core/relay/plugins/traffic/content-enricher-plugin"
	"github.com/fullstorydev/relay-core/relay/test"
	"github.com/fullstorydev/relay-core/relay/traffic"
	"github.com/fullstorydev/relay-core/relay/version"
)

func TestContentEnriching(t *testing.T) {
	testCases := []contentEnricherTestCase{
		{
			desc: "Body content can be enriched with new fields",
			config: `enrich-content:
  body:
    new-body-key: "enrich payload"`,
			originalBody: `{"content":"Original content"}`,
			expectedBody: `{"content":"Original content","new-body-key":"enrich payload"}`,
		},
		{
			desc: "Headers can be enriched with new fields",
			config: `enrich-content:
  headers:
    newhead: "newvalue"`,
			originalHeaders: map[string]string{
				"X-Original-Header": "original value",
			},
			expectedHeaders: map[string]string{
				"X-Original-Header": "original value",
				"newhead":          "newvalue",
			},
		},
		{
			desc: "Both body and headers can be enriched",
			config: `enrich-content:
  body:
    new-body-key: "enrich payload"
  headers:
    newhead: "newvalue"`,
			originalBody: `{"content":"Original content"}`,
			expectedBody: `{"content":"Original content","new-body-key":"enrich payload"}`,
			originalHeaders: map[string]string{
				"X-Original-Header": "original value",
			},
			expectedHeaders: map[string]string{
				"X-Original-Header": "original value",
				"newhead":          "newvalue",
			},
		},
	}

	for _, testCase := range testCases {
		runContentEnricherTest(t, testCase, traffic.Identity)
		runContentEnricherTest(t, testCase, traffic.Gzip)
	}
}

type contentEnricherTestCase struct {
	desc            string
	config          string
	originalBody    string
	expectedBody    string
	originalHeaders map[string]string
	expectedHeaders map[string]string
}

func runContentEnricherTest(t *testing.T, testCase contentEnricherTestCase, encoding traffic.Encoding) {
	var encodingStr string
	switch encoding {
	case traffic.Gzip:
		encodingStr = "gzip"
	case traffic.Identity:
		encodingStr = ""
	}

	desc := fmt.Sprintf("%s (encoding: %v)", testCase.desc, encodingStr)

	plugins := []traffic.PluginFactory{
		content_enricher_plugin.Factory,
	}

	originalHeaders := testCase.originalHeaders
	if originalHeaders == nil {
		originalHeaders = make(map[string]string)
	}

	expectedHeaders := testCase.expectedHeaders
	if expectedHeaders == nil {
		expectedHeaders = make(map[string]string)
	}

	expectedHeaders[content_enricher_plugin.PluginVersionHeaderName] = version.RelayRelease

	test.WithCatcherAndRelay(t, testCase.config, plugins, func(catcherService *catcher.Service, relayService *relay.Service) {
		b, err := traffic.EncodeData([]byte(testCase.originalBody), encoding)
		if err != nil {
			t.Errorf("Test '%v': Error encoding data: %v", desc, err)
			return
		}

		request, err := http.NewRequest(
			"POST",
			relayService.HttpUrl(),
			bytes.NewBuffer(b),
		)
		if err != nil {
			t.Errorf("Test '%v': Error creating request: %v", desc, err)
			return
		}

		if encoding == traffic.Gzip {
			request.Header.Set("Content-Encoding", "gzip")
		}

		request.Header.Set("Content-Type", "application/json")
		for header, headerValue := range originalHeaders {
			request.Header.Set(header, headerValue)
		}

		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Errorf("Test '%v': Error POSTing: %v", desc, err)
			return
		}
		defer response.Body.Close()

		if response.StatusCode != 200 {
			t.Errorf("Test '%v': Expected 200 response: %v", desc, response)
			return
		}

		lastRequest, err := catcherService.LastRequest()
		if err != nil {
			t.Errorf("Test '%v': Error reading last request from catcher: %v", desc, err)
			return
		}

		for expectedHeader, expectedHeaderValue := range expectedHeaders {
			actualHeaderValue := lastRequest.Header.Get(expectedHeader)
			if expectedHeaderValue != actualHeaderValue {
				t.Errorf(
					"Test '%v': Expected header '%v' with value '%v' but got: %v",
					desc,
					expectedHeader,
					expectedHeaderValue,
					actualHeaderValue,
				)
			}
		}

		if lastRequest.Header.Get("Content-Encoding") != encodingStr {
			t.Errorf(
				"Test '%v': Expected Content-Encoding '%v' but got: %v",
				desc,
				encodingStr,
				lastRequest.Header.Get("Content-Encoding"),
			)
		}

		lastRequestBody, err := catcherService.LastRequestBody()
		if err != nil {
			t.Errorf("Test '%v': Error reading last request body from catcher: %v", desc, err)
			return
		}

		contentLength, err := strconv.Atoi(lastRequest.Header.Get("Content-Length"))
		if err != nil {
			t.Errorf("Test '%v': Error parsing Content-Length: %v", desc, err)
			return
		}

		if contentLength != len(lastRequestBody) {
			t.Errorf(
				"Test '%v': Content-Length is %v but actual body length is %v",
				desc,
				contentLength,
				len(lastRequestBody),
			)
		}

		decodedRequestBody, err := traffic.DecodeData(lastRequestBody, encoding)
		if err != nil {
			t.Errorf("Test '%v': Error decoding data: %v", desc, err)
			return
		}

		lastRequestBodyStr := string(decodedRequestBody)
		if testCase.expectedBody != lastRequestBodyStr {
			t.Errorf(
				"Test '%v': Expected body '%v' but got: %v",
				desc,
				testCase.expectedBody,
				lastRequestBodyStr,
			)
		}
	})
}
