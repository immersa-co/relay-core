package plugin_loader

import (
	content_blocker_plugin "github.com/fullstorydev/relay-core/relay/plugins/traffic/content-blocker-plugin"
	content_enricher_plugin "github.com/fullstorydev/relay-core/relay/plugins/traffic/content-enricher-plugin"
	cookies_plugin "github.com/fullstorydev/relay-core/relay/plugins/traffic/cookies-plugin"
	headers_plugin "github.com/fullstorydev/relay-core/relay/plugins/traffic/headers-plugin"
	paths_plugin "github.com/fullstorydev/relay-core/relay/plugins/traffic/paths-plugin"
	test_interceptor_plugin "github.com/fullstorydev/relay-core/relay/plugins/traffic/test-interceptor-plugin"
	"github.com/fullstorydev/relay-core/relay/traffic"
)

// DefaultPlugins is a plugin registry containing all traffic plugins that
// should be available in production. These are the plugins that the relay loads
// on startup.
var DefaultPlugins = []traffic.PluginFactory{
	content_blocker_plugin.Factory,
	content_enricher_plugin.Factory,
	cookies_plugin.Factory,
	headers_plugin.Factory,
	paths_plugin.Factory,
}

// TestPlugins is a plugin registry containing test-only traffic plugins. These
// are not loaded by the relay on startup, but can be loaded programmatically in
// tests.
var TestPlugins = []traffic.PluginFactory{
	test_interceptor_plugin.Factory,
}
