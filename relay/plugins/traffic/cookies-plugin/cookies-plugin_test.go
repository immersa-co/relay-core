package cookies_plugin_test

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/immersa-co/relay-core/catcher"
	"github.com/immersa-co/relay-core/relay"
	cookies_plugin "github.com/immersa-co/relay-core/relay/plugins/traffic/cookies-plugin"
	"github.com/immersa-co/relay-core/relay/test"
	"github.com/immersa-co/relay-core/relay/traffic"
)

func TestRelayedCookies(t *testing.T) {
	testCases := []struct {
		desc                  string
		config                string
		originalCookieHeaders []string
		expectedCookieHeaders []string
	}{
		{
			desc:                  "No cookies are relayed by default",
			originalCookieHeaders: []string{"SPECIAL_ID=298zf09hf012fh2; token=u32t4o3tb3gg43", "_gat=1"},
			expectedCookieHeaders: nil,
		},
		{
			desc: "Multiple Cookie headers are merged",
			config: `cookies:
                        allowlist:
                          - SPECIAL_ID
                          - token
                          - _gat
            `,
			originalCookieHeaders: []string{"SPECIAL_ID=298zf09hf012fh2; token=u32t4o3tb3gg43", "_gat=1"},
			expectedCookieHeaders: []string{"SPECIAL_ID=298zf09hf012fh2; token=u32t4o3tb3gg43; _gat=1"},
		},
		{
			desc: "Only allowlisted cookies are relayed",
			config: `cookies:
                        allowlist:
                          - SPECIAL_ID
                          - foo
                          - _gat
            `,
			originalCookieHeaders: []string{"SPECIAL_ID=298zf09hf012fh2; token=u32t4o3tb3gg43; foo=bar", "_gat=1; bar=foo"},
			expectedCookieHeaders: []string{"SPECIAL_ID=298zf09hf012fh2; foo=bar; _gat=1"},
		},
		{
			desc: "A Cookie header is dropped entirely when no cookies match",
			config: `cookies:
                        allowlist:
                          - bar
            `,
			originalCookieHeaders: []string{"SPECIAL_ID=298zf09hf012fh2; token=u32t4o3tb3gg43; foo=bar", "_gat=1; bar=foo"},
			expectedCookieHeaders: []string{"bar=foo"},
		},
		{
			desc: "TRAFFIC_RELAY_COOKIES syntax is supported",
			config: `cookies:
                        TRAFFIC_RELAY_COOKIES: SPECIAL_ID _gat
            `,
			originalCookieHeaders: []string{"SPECIAL_ID=298zf09hf012fh2; token=u32t4o3tb3gg43; _gat=1"},
			expectedCookieHeaders: []string{"SPECIAL_ID=298zf09hf012fh2; _gat=1"},
		},
		{
			desc: "TRAFFIC_RELAY_COOKIES can be combined with the normal allowlist",
			config: `cookies:
                        allowlist:
                          - safe_cookie
                        TRAFFIC_RELAY_COOKIES: SPECIAL_ID _gat
            `,
			originalCookieHeaders: []string{"SPECIAL_ID=298zf09hf012fh2; token=u32t4o3tb3gg43; _gat=1; safe_cookie=xyz"},
			expectedCookieHeaders: []string{"SPECIAL_ID=298zf09hf012fh2; _gat=1; safe_cookie=xyz"},
		},
	}

	plugins := []traffic.PluginFactory{
		cookies_plugin.Factory,
	}

	for _, testCase := range testCases {
		test.WithCatcherAndRelay(t, testCase.config, plugins, func(catcherService *catcher.Service, relayService *relay.Service) {
			request, err := http.NewRequest("GET", relayService.HttpUrl(), nil)
			if err != nil {
				t.Errorf("Test '%v': Error creating request: %v", testCase.desc, err)
				return
			}

			for _, cookieHeaderValue := range testCase.originalCookieHeaders {
				request.Header.Add("Cookie", cookieHeaderValue)
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

			actualCookieHeaders := lastRequest.Header["Cookie"]
			if !reflect.DeepEqual(testCase.expectedCookieHeaders, actualCookieHeaders) {
				t.Errorf(
					"Test '%v': Expected Cookie header values '%v' but got '%v'",
					testCase.desc,
					testCase.expectedCookieHeaders,
					actualCookieHeaders,
				)
			}
		})
	}
}

/*
Copyright 2022 FullStory, Inc.

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
