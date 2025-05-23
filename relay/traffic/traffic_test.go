package traffic_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/immersa-co/relay-core/catcher"
	"github.com/immersa-co/relay-core/relay"
	test_interceptor_plugin "github.com/immersa-co/relay-core/relay/plugins/traffic/test-interceptor-plugin"
	"github.com/immersa-co/relay-core/relay/test"
	"github.com/immersa-co/relay-core/relay/traffic"
	"github.com/immersa-co/relay-core/relay/version"
	"golang.org/x/net/websocket"
)

func TestBasicRelay(t *testing.T) {
	test.WithCatcherAndRelay(t, "", nil, func(catcherService *catcher.Service, relayService *relay.Service) {
		catcherBody := getBody(catcherService.HttpUrl(), t)
		if catcherBody == nil {
			return
		}

		relayBody := getBody(relayService.HttpUrl(), t)
		if relayBody == nil {
			return
		}

		if bytes.Equal(catcherBody, relayBody) == false {
			t.Errorf("Bodies don't match: \"%v\" \"%v\"", catcherBody, relayBody)
			return
		}
	})
}

func TestRelayedHeaders(t *testing.T) {
	testCases := []struct {
		desc            string
		originalHeaders map[string]string
		expectedHeaders map[string]string
	}{
		{
			desc: "Most headers are relayed by default",
			originalHeaders: map[string]string{
				"Accept-Encoding": "deflate, gzip;q=1.0, *;q=0.5",
				"Downlink":        "100",
				"Origin":          "https://test.com",
				"Viewport-Width":  "100",
			},
			expectedHeaders: map[string]string{
				"Accept-Encoding": "deflate, gzip;q=1.0, *;q=0.5",
				"Downlink":        "100",
				"Origin":          "https://test.com",
				"Viewport-Width":  "100",
			},
		},
		{
			desc: "The Cookie header is not relayed by default",
			originalHeaders: map[string]string{
				"Cookie": "TOKEN=xyz123",
			},
			expectedHeaders: map[string]string{},
		},
	}

	for _, testCase := range testCases {
		var lastClientIP, lastClientPort string

		plugins := []traffic.PluginFactory{
			test_interceptor_plugin.NewFactoryWithListener(func(request *http.Request) {
				// Capture the actual IP and port used in the request.
				addrComponents := strings.Split(request.RemoteAddr, ":")
				lastClientIP = addrComponents[0]
				lastClientPort = addrComponents[1]
			}),
		}

		test.WithCatcherAndRelay(t, "", plugins, func(catcherService *catcher.Service, relayService *relay.Service) {
			request, err := http.NewRequest("GET", relayService.HttpUrl(), nil)
			if err != nil {
				t.Errorf("Test '%v': Error creating request: %v", testCase.desc, err)
				return
			}

			for headerName, headerValue := range testCase.originalHeaders {
				request.Header.Add(headerName, headerValue)
			}

			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Errorf("Test '%v': Error GETing: %v", testCase.desc, err)
				return
			}
			defer response.Body.Close()

			if response.StatusCode != 200 {
				t.Errorf("Test '%v': Expected 200 response: %v", testCase.desc, response)
				return
			}

			lastRequest, err := catcherService.LastRequest()
			if err != nil {
				t.Errorf("Test '%v': Error reading last request from catcher: %v", testCase.desc, err)
				return
			}

			// Check for the built-in headers that the relay always generates.
			testCase.expectedHeaders["X-Forwarded-Proto"] = "http"
			testCase.expectedHeaders[traffic.RelayVersionHeaderName] = version.RelayRelease
			testCase.expectedHeaders["X-Forwarded-For"] = lastClientIP
			testCase.expectedHeaders["X-Forwarded-Port"] = lastClientPort

			for headerName, expectedHeaderValue := range testCase.expectedHeaders {
				expectedHeaderValues := []string{expectedHeaderValue}
				actualHeaderValues := lastRequest.Header[headerName]
				if !reflect.DeepEqual(expectedHeaderValues, actualHeaderValues) {
					t.Errorf(
						"Test '%v': Expected '%v' header values '%v' but got '%v'",
						testCase.desc,
						headerName,
						expectedHeaderValues,
						actualHeaderValues,
					)
				}
			}
		})
	}
}

func TestMaxBodySize(t *testing.T) {
	configYaml := `relay:
                      max-body-size: 5
    `

	test.WithCatcherAndRelay(t, configYaml, nil, func(catcherService *catcher.Service, relayService *relay.Service) {
		response, err := http.Get(relayService.HttpUrl())
		if err != nil {
			t.Errorf("Error GETing: %v", err)
			return
		}
		defer response.Body.Close()
		if response.StatusCode != 503 {
			t.Errorf("Expected 503 response for surpassing max body size: %v", response)
			return
		}
	})
}

func TestRelaySupportsContentEncoding(t *testing.T) {
	testCases := map[string]struct {
		encoding       traffic.Encoding
		bodyContentStr string
		headers        map[string]string
		customUrl      func(relayServiceURL string) string
	}{
		"identity": {
			encoding:       traffic.Identity,
			bodyContentStr: "Hello, world!",
		},
		"gzip - with header": {
			encoding:       traffic.Gzip,
			bodyContentStr: "Hello, world!",
			headers: map[string]string{
				"Content-Encoding": "gzip",
			},
		},
		"gzip - with query param": {
			encoding:       traffic.Gzip,
			bodyContentStr: "Hello, world!",
			customUrl: func(relayServiceURL string) string {
				return fmt.Sprintf("%v?ContentEncoding=gzip", relayServiceURL)
			},
		},
	}

	for desc, testCase := range testCases {
		test.WithCatcherAndRelay(t, "", nil, func(catcherService *catcher.Service, relayService *relay.Service) {
			// convert the body content to a reader with the proper content encoding applied
			var body io.Reader
			switch testCase.encoding {
			case traffic.Gzip:
				b, err := traffic.EncodeData([]byte(testCase.bodyContentStr), traffic.Gzip)
				if err != nil {
					t.Errorf("Test %s - Error encoding data: %v", desc, err)
					return
				}
				body = bytes.NewReader(b)
			case traffic.Identity:
				body = strings.NewReader(testCase.bodyContentStr)
			}

			requestURL := relayService.HttpUrl()
			if testCase.customUrl != nil {
				requestURL = testCase.customUrl(requestURL)
			}
			request, err := http.NewRequest("POST", requestURL, body)
			if err != nil {
				t.Errorf("Test %s - Error GETing: %v", desc, err)
				return
			}

			for header, headerValue := range testCase.headers {
				request.Header.Set(header, headerValue)
			}

			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Errorf("Test %s - Error POSTing: %v", desc, err)
				return
			}

			defer response.Body.Close()

			if response.StatusCode != 200 {
				t.Errorf("Test %s - Expected 200 response: %v", desc, response)
				return
			}

			lastRequest, err := catcherService.LastRequestBody()
			if err != nil {
				t.Errorf("Test %s - Error reading last request body from catcher: %v", desc, err)
				return
			}

			switch testCase.encoding {
			case traffic.Gzip:
				decodedData, err := traffic.DecodeData(lastRequest, traffic.Gzip)
				if err != nil {
					t.Errorf("Test %s - Error decoding data: %v", desc, err)
					return
				}
				if string(decodedData) != testCase.bodyContentStr {
					t.Errorf("Test %s - Expected body '%v' but got: %v", desc, testCase.bodyContentStr, string(decodedData))
				}
			case traffic.Identity:
				if string(lastRequest) != testCase.bodyContentStr {
					t.Errorf("Test %s - Expected body '%v' but got: %v", desc, testCase.bodyContentStr, string(lastRequest))
				}
			}
		})
	}
}

func TestRelayNotFound(t *testing.T) {
	test.WithCatcherAndRelay(t, "", nil, func(catcherService *catcher.Service, relayService *relay.Service) {
		faviconURL := fmt.Sprintf("%v/favicon.ico", relayService.HttpUrl())
		response, err := http.Get(faviconURL)
		if err != nil {
			t.Errorf("Error GETing: %v", err)
			return
		}
		if response.StatusCode != 404 {
			t.Errorf("Should have received 404: %v", response)
			return
		}
	})
}

func TestWebSocketEcho(t *testing.T) {
	test.WithCatcherAndRelay(t, "", nil, func(catcherService *catcher.Service, relayService *relay.Service) {
		echoURL := fmt.Sprintf("%v/echo", relayService.WsUrl())
		ws, err := websocket.Dial(echoURL, "", relayService.HttpUrl())
		if err != nil {
			t.Errorf("Error dialing websocket: %v", err)
			return
		}
		err = testEcho(ws, "Come in, good buddy")
		if err != nil {
			t.Errorf("Error in echo: %v", err)
			return
		}
		err = testEcho(ws, "10-4, Rocket")
		if err != nil {
			t.Errorf("Error in second echo: %v", err)
			return
		}
	})
}

func testEcho(conn *websocket.Conn, message string) error {
	_, err := conn.Write([]byte(message))
	if err != nil {
		return err
	}
	var response = make([]byte, len(message)+10)
	n, err := conn.Read(response)
	if err != nil {
		return err
	}
	if strings.Compare(message, string(response[:n])) != 0 {
		return errors.New(fmt.Sprintf("Unexpected echo response: %v", string(response[:n])))
	}
	return nil
}

func getBody(url string, t *testing.T) []byte {
	response, err := http.Get(url)
	if err != nil {
		t.Errorf("Error GETing: %v", err)
		return nil
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Errorf("Non-200 GET: %v", response)
		return nil
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Errorf("Error GETing body: %v", err)
		return nil
	}
	return body
}
