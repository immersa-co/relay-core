package test

import (
	"testing"

	"github.com/immersa-co/relay-core/catcher"
	"github.com/immersa-co/relay-core/relay"
	"github.com/immersa-co/relay-core/relay/config"
	"github.com/immersa-co/relay-core/relay/traffic"
	plugin_loader "github.com/immersa-co/relay-core/relay/traffic/plugin-loader"
)

// WithCatcherAndRelay is a helper function that wraps the setup and teardown
// required by most relay unit tests. Given a configuration and a list of
// plugins to load, it loads and configures the plugins, starts the catcher and
// relay services, and invokes the provided action function, which should handle
// the actual testing. Afterwards, it ensures that everything gets torn down so
// that the next test can start from a clean slate.
func WithCatcherAndRelay(
	t *testing.T,
	configYaml string,
	pluginFactories []traffic.PluginFactory,
	action func(catcherService *catcher.Service, relayService *relay.Service),
) {
	catcherService := catcher.NewService()
	if err := catcherService.Start("localhost", 0); err != nil {
		t.Errorf("Error starting catcher: %v", err)
		return
	}
	defer catcherService.Close()

	configFile, err := config.NewFileFromYamlString(configYaml)
	if err != nil {
		t.Errorf("Error parsing configuration YAML: %v", err)
		return
	}

	relaySection := configFile.GetOrAddSection("relay")
	relaySection.Set("port", 0)
	relaySection.Set("target", catcherService.HttpUrl())

	relayService, err := setupRelay(configFile, pluginFactories)
	if err != nil {
		t.Errorf("Error setting up relay: %v", err)
		return
	}

	if err := relayService.Start("localhost", 0); err != nil {
		t.Errorf("Error starting relay: %v", err)
		return
	}
	defer relayService.Close()

	action(catcherService, relayService)
}

func setupRelay(
	configFile *config.File,
	pluginFactories []traffic.PluginFactory,
) (*relay.Service, error) {
	options, err := relay.ReadOptions(configFile)
	if err != nil {
		return nil, err
	}

	trafficPlugins, err := plugin_loader.Load(pluginFactories, configFile)
	if err != nil {
		return nil, err
	}

	return relay.NewService(options.Relay, trafficPlugins), nil
}
