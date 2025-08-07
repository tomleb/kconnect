/*
Copyright 2020 The kconnect Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package rancher

import (
	"fmt"
	"strings"

	"go.uber.org/zap"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/fidelity/kconnect/pkg/config"
	khttp "github.com/fidelity/kconnect/pkg/http"
	"github.com/fidelity/kconnect/pkg/provider"
	"github.com/fidelity/kconnect/pkg/provider/common"
	"github.com/fidelity/kconnect/pkg/provider/discovery"
	"github.com/fidelity/kconnect/pkg/provider/identity"
	"github.com/fidelity/kconnect/pkg/provider/registry"
	rshared "github.com/fidelity/kconnect/pkg/rancher"
)

const (
	ProviderName = "rancher"
	UsageExample = `  # Discover Rancher clusters using Active Directory
	{{.CommandPath}} use rancher --idp-protocol rancher-ad

	# Discover clusters via Rancher using a API key
	{{.CommandPath}} use rancher --idp-protocol static-token --token ABCDEF
  `
)

func init() {
	if err := registry.RegisterDiscoveryPlugin(&registry.DiscoveryPluginRegistration{
		PluginRegistration: registry.PluginRegistration{
			Name:                   ProviderName,
			UsageExample:           UsageExample,
			ConfigurationItemsFunc: ConfigurationItems,
		},
		CreateFunc:                 New,
		SupportedIdentityProviders: []string{"static-token", "rancher-ad"},
	}); err != nil {
		zap.S().Fatalw("Failed to register Rancher discovery plugin", "error", err)
	}
}

// New will create a new Rancher discovery plugin
func New(input *provider.PluginCreationInput) (discovery.Provider, error) {
	if input.HTTPClient == nil {
		return nil, provider.ErrHTTPClientRequired
	}

	return &rancherClusterProvider{
		logger:      input.Logger,
		interactive: input.IsInteractive,
		httpClient:  input.HTTPClient,
	}, nil
}

type rancherClusterProviderConfig struct {
	common.ClusterProviderConfig
	rshared.CommonConfig
	rshared.UseConfig
}

type rancherClusterProvider struct {
	config     *rancherClusterProviderConfig
	token      string
	restConfig *rest.Config

	httpClient  khttp.Client
	interactive bool
	logger      *zap.SugaredLogger
}

func (p *rancherClusterProvider) Name() string {
	return ProviderName
}

func (p *rancherClusterProvider) setup(cs config.ConfigurationSet, userID identity.Identity) error {
	cfg := &rancherClusterProviderConfig{}
	if err := config.Unmarshall(cs, cfg); err != nil {
		return fmt.Errorf("unmarshalling config items into rancherClusterProviderConfig: %w", err)
	}
	p.config = cfg

	id, ok := userID.(*identity.TokenIdentity)
	if !ok {
		return identity.ErrNotTokenIdentity
	}
	p.token = id.Token()
	restConfig, err := generateKubeconfigFromToken(p.config.APIEndpoint, p.token)
	if err != nil {
		return err
	}
	p.restConfig = restConfig

	return nil
}

// Generate the kubeconfig as described in https://ranchermanager.docs.rancher.com/api/quickstart
func generateKubeconfigFromToken(apiEndpoint, token string) (*rest.Config, error) {
	url := strings.TrimRight(apiEndpoint, "/v3")
	kubeconfig := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: "rancher"
  cluster:
    server: "%s"

users:
- name: "rancher"
  user:
    token: "%s"

contexts:
- name: "rancher"
  context:
    user: "rancher"
    cluster: "rancher"

current-context: "rancher"
`, url, token)
	fmt.Println(kubeconfig)
	clientConfig, err := clientcmd.NewClientConfigFromBytes([]byte(kubeconfig))
	if err != nil {
		return nil, err
	}

	return clientConfig.ClientConfig()
}

func (p *rancherClusterProvider) ListPreReqs() []*provider.PreReq {
	return []*provider.PreReq{}
}

func (p *rancherClusterProvider) CheckPreReqs() error {
	return nil
}

func ConfigurationItems(scopeTo string) (config.ConfigurationSet, error) {
	cs := config.NewConfigurationSet()
	rshared.AddCommonConfig(cs) //nolint: errcheck
	rshared.AddUseConfig(cs)    //nolint: errcheck
	return cs, nil
}
