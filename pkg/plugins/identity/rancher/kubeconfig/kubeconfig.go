package kubeconfig

import (
	"context"
	"fmt"

	"github.com/go-playground/validator/v10"
	extv1 "github.com/rancher/rancher/pkg/apis/ext.cattle.io/v1"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/fidelity/kconnect/pkg/config"
	"github.com/fidelity/kconnect/pkg/prompt"
	"github.com/fidelity/kconnect/pkg/provider"
	"github.com/fidelity/kconnect/pkg/provider/identity"
	"github.com/fidelity/kconnect/pkg/provider/registry"
)

const (
	ProviderName         = "rancher-kubeconfig"
	tokenConfigItem      = "token"
	kubeconfigConfigItem = "kubeconfig-main"
)

func init() {
	if err := registry.RegisterIdentityPlugin(&registry.IdentityPluginRegistration{
		PluginRegistration: registry.PluginRegistration{
			Name:                   ProviderName,
			UsageExample:           "",
			ConfigurationItemsFunc: ConfigurationItems,
		},
		CreateFunc: New,
	}); err != nil {
		zap.S().Fatalw("Failed to register rancher kubeconfig identity plugin", "error", err)
	}
}

// New will create a new static token identity provider
func New(input *provider.PluginCreationInput) (identity.Provider, error) {
	return &kubeconfigIdentityProvider{
		logger:      input.Logger,
		interactive: input.IsInteractive,
	}, nil
}

type kubeconfigIdentityProvider struct {
	logger      *zap.SugaredLogger
	interactive bool
}

type providerConfig struct {
	Token          string `json:"token" validate:"required"`
	KubeconfigPath string `json:"kubeconfig-main" validate:"required"`
}

func (p *kubeconfigIdentityProvider) Name() string {
	return ProviderName
}

// Authenticate will authenticate a user and return details of their identity.
func (p *kubeconfigIdentityProvider) Authenticate(ctx context.Context, input *identity.AuthenticateInput) (*identity.AuthenticateOutput, error) {
	p.logger.Info("using static token for authentication")

	if err := p.resolveConfig(input.ConfigSet); err != nil {
		return nil, fmt.Errorf("resolving config: %w", err)
	}

	cfg := &providerConfig{}
	if err := config.Unmarshall(input.ConfigSet, cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config into providerConfig: %w", err)
	}

	if err := p.validateConfig(cfg); err != nil {
		return nil, err
	}

	restConfig, err := clientcmd.BuildConfigFromFlags("", cfg.KubeconfigPath)
	if err != nil {
		return nil, err
	}

	return &identity.AuthenticateOutput{
		Identity: identity.NewKubeconfigIdentity("kubeconfig", cfg.Token, ProviderName, restConfig),
	}, nil

}

func (p *kubeconfigIdentityProvider) validateConfig(cfg *providerConfig) error {
	validate := validator.New()
	if err := validate.Struct(cfg); err != nil {
		return fmt.Errorf("validating aad config: %w", err)
	}
	return nil
}

func (p *kubeconfigIdentityProvider) resolveConfig(cfg config.ConfigurationSet) error {
	if !p.interactive {
		p.logger.Debug("skipping configuration resolution as runnning non-interactive")
	}

	if err := prompt.InputAndSet(cfg, kubeconfigConfigItem, "Enter kubeconfig path", true); err != nil {
		return fmt.Errorf("resolving %s: %w", kubeconfigConfigItem, err)
	}

	restConfig, err := clientcmd.BuildConfigFromFlags("", cfg.ValueString(kubeconfigConfigItem))
	if err != nil {
		return err
	}

	dynClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return err
	}

	// Generate a Kubeconfig as described in https://ranchermanager.docs.rancher.com/api/workflows/kubeconfigs#creating-a-kubeconfig
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "ext.cattle.io/v1",
			"kind":       "Token",
		},
	}
	gvr := extv1.SchemeGroupVersion.WithResource("tokens")
	resp, err := dynClient.Resource(gvr).Create(context.TODO(), obj, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create kubeconfig: %w", err)
	}

	tokenName, _, _ := unstructured.NestedString(resp.Object, "metadata", "name")
	token, found, err := unstructured.NestedString(resp.Object, "status", "value")
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("tokens not available")
	}

	// Generate a valid bearer token to keep using legacy API
	// TODO: This is not documented anywhere yet, we need to add this to docs
	cfg.SetValue(tokenConfigItem, fmt.Sprintf("ext/%s:%s", tokenName, token))
	return nil
}

// ConfigurationItems will return the configuration items for the intentity plugin based
// of the cluster provider that its being used in conjunction with
func ConfigurationItems(scopeTo string) (config.ConfigurationSet, error) {
	cs := config.NewConfigurationSet()

	// token is fetched from the kubeconfig
	cs.String(tokenConfigItem, "", "the token to use for authentication") //nolint:errcheck
	cs.SetRequired(tokenConfigItem)                                       //nolint:errcheck
	cs.SetSensitive(tokenConfigItem)                                      //nolint:errcheck
	cs.SetHidden(tokenConfigItem)                                         //nolint:errcheck

	cs.String(kubeconfigConfigItem, "", "the kubeconfig path") //nolint:errcheck
	cs.SetRequired(kubeconfigConfigItem)                       //nolint:errcheck
	cs.SetSensitive(kubeconfigConfigItem)                      //nolint:errcheck

	return cs, nil
}
