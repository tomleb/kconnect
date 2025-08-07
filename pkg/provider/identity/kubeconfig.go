package identity

import (
	"errors"

	"k8s.io/client-go/rest"
)

var (
	ErrNotKubeconfigIdentity = errors.New("not a kubeconfig identity")
)

type KubeconfigIdentity struct {
	token          string
	name           string
	idProviderName string
	restConfig     *rest.Config
}

func NewKubeconfigIdentity(name, token, idProviderName string, restConfig *rest.Config) *KubeconfigIdentity {
	return &KubeconfigIdentity{
		token:          token,
		restConfig:     restConfig,
		name:           name,
		idProviderName: idProviderName,
	}
}

func (t *KubeconfigIdentity) Type() string {
	return "kubeconfig"
}

func (t *KubeconfigIdentity) Name() string {
	return t.name
}

func (t *KubeconfigIdentity) IsExpired() bool {
	// TODO: handle properly
	return false
}

func (t *KubeconfigIdentity) IdentityProviderName() string {
	return t.idProviderName
}

func (t *KubeconfigIdentity) Token() string {
	return t.token
}

func (t *KubeconfigIdentity) RestConfig() *rest.Config {
	return t.restConfig
}
