package wait

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/k8s"
	"github.com/shopware/shopware-operator/internal/manager/base"
	"github.com/shopware/shopware-operator/internal/util"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type Manager struct {
	*base.Base
}

func New(b *base.Base) *Manager {
	return &Manager{Base: b}
}

func (m *Manager) StateHandler(ctx context.Context, store *v1.Store) v1.StatefulAppState {
	// We disable the checks for local development, so you don't need to run
	// portforwards or dns. This is later also important if we plan to install one
	// operator for multiple namespaces.
	if m.DisableServiceChecks || store.Spec.DisableChecks {
		return v1.StateSetup
	}

	if store.IsState(v1.StateEmpty) {
		return v1.StateWait
	}

	next := v1.StateSetup

	if !store.Spec.DisableDatabaseCheck {
		if state := m.checkDatabaseServices(ctx, store); state != v1.StateSetup {
			next = state
		}
	}

	if !store.Spec.DisableS3Check && store.Spec.S3Storage.AccessKeyRef.Key != "" {
		if state := m.checkS3Services(ctx, store); state != v1.StateSetup {
			next = state
		}
	}

	if !store.Spec.DisableFastlyCheck && store.Spec.ShopConfiguration.Fastly.ServiceRef.Name != "" && store.Spec.ShopConfiguration.Fastly.ServiceRef.Key != "" {
		if state := m.checkFastlyRef(ctx, store); state != v1.StateSetup {
			next = state
		}
	}

	if !store.Spec.DisableOpensearchCheck && store.Spec.OpensearchSpec.Enabled {
		if state := m.checkOpensearch(ctx, store); state != v1.StateSetup {
			next = state
		}
	}

	return next
}

func (m *Manager) ResourceHandler(_ context.Context, _ *v1.Store) error {
	return nil
}

func (m *Manager) checkDatabaseServices(
	ctx context.Context,
	store *v1.Store,
) v1.StatefulAppState {
	con := v1.StoreCondition{
		Type:               string(v1.StateWait),
		LastTransitionTime: metav1.Time{},
		LastUpdateTime:     metav1.Now(),
		Message:            "Waiting for database connection",
		Reason:             "",
		Status:             "",
	}
	defer func() {
		store.Status.AddCondition(con)
	}()

	dbSpec, err := util.GetDBSpec(ctx, *store, m.Client)
	if err != nil {
		con.Reason = err.Error()
		con.Status = base.Error
		return v1.StateWait
	}

	err = util.TestSQLConnection(ctx, dbSpec)
	if err != nil {
		con.Reason = err.Error()
		con.Status = base.Error
		return v1.StateWait
	}

	con.LastTransitionTime = metav1.Now()
	con.Status = base.Ready
	con.Reason = "Database ping passed"
	return v1.StateSetup
}

func (m *Manager) checkFastlyRef(
	ctx context.Context,
	store *v1.Store,
) v1.StatefulAppState {
	con := v1.StoreCondition{
		Type:               string(v1.StateWait),
		LastTransitionTime: metav1.Time{},
		LastUpdateTime:     metav1.Now(),
		Message:            "Waiting for fastly secret",
		Reason:             "",
		Status:             "",
	}
	defer func() {
		store.Status.AddCondition(con)
	}()

	fastlyServiceIDSecret := new(corev1.Secret)
	if err := m.Get(ctx, types.NamespacedName{
		Namespace: store.Namespace,
		Name:      store.Spec.ShopConfiguration.Fastly.ServiceRef.Name,
	}, fastlyServiceIDSecret); err != nil {
		if k8serrors.IsNotFound(err) {
			con.Status = base.Error
			con.Reason = "Fastly serviceRef secret does not exist"
			return v1.StateWait
		}
		con.Reason = err.Error()
		con.Status = base.Error
		return v1.StateWait
	}

	if _, ok := fastlyServiceIDSecret.Data[store.Spec.ShopConfiguration.Fastly.ServiceRef.Key]; !ok {
		con.Reason = fmt.Sprintf(
			"The ServiceKeyRef doesn't contain the specified key '%s' in the secret '%s'",
			store.Spec.ShopConfiguration.Fastly.ServiceRef.Key,
			store.Spec.ShopConfiguration.Fastly.ServiceRef.Name,
		)
		con.Status = base.Error
		return v1.StateWait
	}

	fastlyTokenSecret := new(corev1.Secret)
	if err := m.Get(ctx, types.NamespacedName{
		Namespace: store.Namespace,
		Name:      store.Spec.ShopConfiguration.Fastly.TokenRef.Name,
	}, fastlyTokenSecret); err != nil {
		if k8serrors.IsNotFound(err) {
			con.Status = base.Error
			con.Reason = "Fastly tokenRef secret does not exist"
			return v1.StateWait
		}
		con.Reason = err.Error()
		con.Status = base.Error
		return v1.StateWait
	}

	if _, ok := fastlyTokenSecret.Data[store.Spec.ShopConfiguration.Fastly.TokenRef.Key]; !ok {
		con.Reason = fmt.Sprintf(
			"The TokenKeyRef doesn't contain the specified key '%s' in the secret '%s'",
			store.Spec.ShopConfiguration.Fastly.TokenRef.Key,
			store.Spec.ShopConfiguration.Fastly.TokenRef.Name,
		)
		con.Status = base.Error
		return v1.StateWait
	}

	con.LastTransitionTime = metav1.Now()
	con.Status = base.Ready
	con.Reason = "Fastly ServiceRef/TokenRef present"
	return v1.StateSetup
}

func (m *Manager) checkOpensearch(
	ctx context.Context,
	store *v1.Store,
) v1.StatefulAppState {
	con := v1.StoreCondition{
		Type:               string(v1.StateWait),
		LastTransitionTime: metav1.Time{},
		LastUpdateTime:     metav1.Now(),
		Message:            "Waiting for Opensearch ref",
		Reason:             "",
		Status:             "",
	}
	defer func() {
		store.Status.AddCondition(con)
	}()

	es := new(corev1.Secret)
	if err := m.Get(ctx, types.NamespacedName{
		Namespace: store.Namespace,
		Name:      store.Spec.OpensearchSpec.PasswordSecretRef.Name,
	}, es); err != nil {
		if k8serrors.IsNotFound(err) {
			con.Status = base.Error
			con.Reason = "OpensearchRef secret does not exist"
			return v1.StateWait
		}
		con.Reason = err.Error()
		con.Status = base.Error
		return v1.StateWait
	}

	if _, ok := es.Data[store.Spec.OpensearchSpec.PasswordSecretRef.Key]; !ok {
		con.Reason = fmt.Sprintf(
			"The SecretKeyRef doesn't contain the specified key '%s' in the secret '%s'",
			store.Spec.OpensearchSpec.PasswordSecretRef.Key,
			store.Spec.OpensearchSpec.PasswordSecretRef.Name,
		)
		con.Status = base.Error
		return v1.StateWait
	}

	con.LastTransitionTime = metav1.Now()
	con.Status = base.Ready
	con.Reason = "OpensearchRef is present"
	return v1.StateSetup
}

func (m *Manager) checkS3Services(
	ctx context.Context,
	store *v1.Store,
) v1.StatefulAppState {
	con := v1.StoreCondition{
		Type:               string(v1.StateWait),
		LastTransitionTime: metav1.Time{},
		LastUpdateTime:     metav1.Now(),
		Message:            "Waiting for s3 connection",
		Reason:             "",
		Status:             "",
	}
	defer func() {
		store.Status.AddCondition(con)
	}()

	secret, err := k8s.GetSecret(ctx, m.Client, types.NamespacedName{
		Namespace: store.Namespace,
		Name:      store.Spec.S3Storage.SecretAccessKeyRef.Name,
	})
	if err != nil {
		con.Reason = err.Error()
		con.Status = base.Error
		return v1.StateWait
	}

	var ok bool
	var secretAccessKey []byte
	if secretAccessKey, ok = secret.Data[store.Spec.S3Storage.SecretAccessKeyRef.Key]; !ok {
		con.Reason = fmt.Sprintf(
			"The SecretAccessKeyRef doesn't contain the specified key '%s' in the secret '%s'",
			store.Spec.S3Storage.SecretAccessKeyRef.Key,
			store.Spec.S3Storage.SecretAccessKeyRef.Name,
		)
		con.Status = base.Error
		return v1.StateWait
	}

	secret, err = k8s.GetSecret(ctx, m.Client, types.NamespacedName{
		Namespace: store.Namespace,
		Name:      store.Spec.S3Storage.AccessKeyRef.Name,
	})
	if err != nil {
		con.Reason = err.Error()
		con.Status = base.Error
		return v1.StateWait
	}

	var accessKey []byte
	if accessKey, ok = secret.Data[store.Spec.S3Storage.AccessKeyRef.Key]; !ok {
		con.Reason = fmt.Sprintf(
			"The AccessKeyRef doesn't contain the specified key '%s' in the secret '%s'",
			store.Spec.S3Storage.AccessKeyRef.Key,
			store.Spec.S3Storage.AccessKeyRef.Name,
		)
		con.Status = base.Error
		return v1.StateWait
	}

	err = util.TestS3Connection(ctx, store.Spec.S3Storage, aws.Credentials{
		AccessKeyID:     string(accessKey),
		SecretAccessKey: string(secretAccessKey),
	})
	if err != nil {
		con.Reason = err.Error()
		con.Status = base.Error
		return v1.StateWait
	}

	con.LastTransitionTime = metav1.Now()
	con.Status = base.Ready
	con.Reason = "S3 connection test passed"
	return v1.StateSetup
}
