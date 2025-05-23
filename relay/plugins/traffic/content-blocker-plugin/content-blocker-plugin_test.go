package content_blocker_plugin_test

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/immersa-co/relay-core/catcher"
	"github.com/immersa-co/relay-core/relay"
	content_blocker_plugin "github.com/immersa-co/relay-core/relay/plugins/traffic/content-blocker-plugin"
	"github.com/immersa-co/relay-core/relay/test"
	"github.com/immersa-co/relay-core/relay/traffic"
	"github.com/immersa-co/relay-core/relay/version"
)

func TestContentBlocking(t *testing.T) {
	testCases := []contentBlockerTestCase{
		{
			desc: "Body content can be excluded",
			config: `block-content:
                        body:
                          - exclude: '[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+'
            `,
			originalBody: `{ "content": "Excluded IP address = 215.1.0.335." }`,
			expectedBody: `{ "content": "Excluded IP address = ." }`,
		},
		{
			desc: "Body content can be masked",
			config: `block-content:
                        body:
                          - mask: '[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+'
            `,
			originalBody: `{ "content": "Excluded IP address = 215.1.0.335." }`,
			expectedBody: `{ "content": "Excluded IP address = ***********." }`,
		},
		{
			desc: "Header content can be excluded",
			config: `block-content:
                        header:
                          - exclude: '[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+'
            `,
			originalHeaders: map[string]string{
				"X-Forwarded-For": "foo.com,192.168.0.1,bar.com",
			},
			expectedHeaders: map[string]string{
				"X-Forwarded-For": "foo.com,,bar.com",
			},
		},
		{
			desc: "Header content can be masked",
			config: `block-content:
                        header:
                          - mask: '[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+'
            `,
			originalHeaders: map[string]string{
				"X-Forwarded-For": "foo.com,192.168.0.1,bar.com",
			},
			expectedHeaders: map[string]string{
				"X-Forwarded-For": "foo.com,***********,bar.com",
			},
		},
		{
			desc: "Header values are blocked but header names are not",
			config: `block-content:
                        header:
                          - exclude: '(?i)BAR'
                          - mask: '(?i)FOO'
            `,
			originalHeaders: map[string]string{
				"X-Barrier":  "foo bar baz",
				"X-Football": "foo bar baz",
			},
			expectedHeaders: map[string]string{
				"X-Barrier":  "***  baz",
				"X-Football": "***  baz",
			},
		},
		{
			desc: "Exclusion takes priority over masking",
			config: `block-content:
                        body:
                          - exclude: '[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+'
                          - mask: '[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+'
            `,
			originalBody: `{ "content": "Excluded IP address = 215.1.0.335." }`,
			expectedBody: `{ "content": "Excluded IP address = ." }`,
		},
		{
			desc: "Complex configurations are supported",
			config: `block-content:
                        body:
                          - exclude: '(?i)EXCLUDED'
                          - mask: '[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+'
                        header:
                          - exclude: '(?i)DELETED'
                          - mask: '(foo|bar)'
            `,
			originalBody: `{ "content": "Excluded, deleted foo bar IP address = 215.1.0.335." }`,
			expectedBody: `{ "content": ", deleted foo bar IP address = ***********." }`,
			originalHeaders: map[string]string{
				"X-Forwarded-For":  "192.168.0.1",
				"X-Headerfoobar":   "bar foo baz bar baz foobar",
				"X-Special-Header": "Some EXCLUDED, DELETED content",
			},
			expectedHeaders: map[string]string{
				"X-Forwarded-For":  "192.168.0.1",
				"X-Headerfoobar":   "*** *** baz *** baz ******",
				"X-Special-Header": "Some EXCLUDED,  content",
			},
		},
		{
			desc: "TRAFFIC_EXCLUDE_* and TRAFFIC_MASK_* are supported",
			config: `block-content:
                        TRAFFIC_EXCLUDE_BODY_CONTENT: '(?i)EXCLUDED'
                        TRAFFIC_MASK_BODY_CONTENT: '[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+'
                        TRAFFIC_EXCLUDE_HEADER_CONTENT: '(?i)DELETED'
                        TRAFFIC_MASK_HEADER_CONTENT: '(foo|bar)'
            `,
			originalBody: `{ "content": "Excluded, deleted foo bar IP address = 215.1.0.335." }`,
			expectedBody: `{ "content": ", deleted foo bar IP address = ***********." }`,
			originalHeaders: map[string]string{
				"X-Forwarded-For":  "192.168.0.1",
				"X-Headerfoobar":   "bar foo baz bar baz foobar",
				"X-Special-Header": "Some EXCLUDED, DELETED content",
			},
			expectedHeaders: map[string]string{
				"X-Forwarded-For":  "192.168.0.1",
				"X-Headerfoobar":   "*** *** baz *** baz ******",
				"X-Special-Header": "Some EXCLUDED,  content",
			},
		},
	}

	for _, testCase := range testCases {
		runContentBlockerTest(t, testCase, traffic.Identity)
		runContentBlockerTest(t, testCase, traffic.Gzip)
	}
}

func TestBlockPluginBlocksWebsockets(t *testing.T) {
	config := `block-content:
                  body:
                    - mask: '[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+'
    `
	plugins := []traffic.PluginFactory{
		content_blocker_plugin.Factory,
	}

	test.WithCatcherAndRelay(t, config, plugins, func(catcherService *catcher.Service, relayService *relay.Service) {
		request, err := http.NewRequest(
			"POST",
			relayService.HttpUrl(),
			bytes.NewBufferString(`{ "content": "192.168.0.1" }`),
		)
		if err != nil {
			t.Errorf("Error creating request: %v", err)
			return
		}

		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Upgrade", "websocket")

		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Errorf("Error POSTing: %v", err)
			return
		}
		defer response.Body.Close()

		// This plugin doesn't support websockets, so we should fail closed and
		// the attempt to establish a websocket connection should fail.
		if response.StatusCode != 500 {
			t.Errorf("Expected 500 response: %v", response)
			return
		}
	})
}

type contentBlockerTestCase struct {
	desc            string
	config          string
	originalBody    string
	expectedBody    string
	originalHeaders map[string]string
	expectedHeaders map[string]string
}

func runContentBlockerTest(t *testing.T, testCase contentBlockerTestCase, encoding traffic.Encoding) {
	var encodingStr string
	switch encoding {
	case traffic.Gzip:
		encodingStr = "gzip"
	case traffic.Identity:
		encodingStr = ""
	}

	// Add encoding to the test description
	desc := fmt.Sprintf("%s (encoding: %v)", testCase.desc, encodingStr)

	plugins := []traffic.PluginFactory{
		content_blocker_plugin.Factory,
	}

	originalHeaders := testCase.originalHeaders
	if originalHeaders == nil {
		originalHeaders = make(map[string]string)
	}

	expectedHeaders := testCase.expectedHeaders
	if expectedHeaders == nil {
		expectedHeaders = make(map[string]string)
	}

	expectedHeaders[content_blocker_plugin.PluginVersionHeaderName] = version.RelayRelease

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
