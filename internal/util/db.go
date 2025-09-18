package util

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os/exec"

	"github.com/go-sql-driver/mysql"
	v1 "github.com/shopware/shopware-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GenerateDatabaseURLForShopware(spec *DatabaseSpec) []byte {
	urlP := url.QueryEscape(string(spec.Password))

	var options string
	if spec.Options != "" {
		options = "&" + spec.Options
	}

	mode := spec.SSLMode
	if spec.SSLMode == "" {
		mode = "PREFERRED"
	}

	plain := fmt.Sprintf(
		"mysql://%s:%s@%s:%d/%s?serverVersion=%s&sslMode=%s%s",
		spec.User,
		urlP,
		spec.Host,
		spec.Port,
		spec.Name,
		spec.Version,
		mode,
		options,
	)
	return []byte(plain)
}

func GenerateDatabaseURLForGo(spec *DatabaseSpec) []byte {
	mode := spec.SSLMode
	if spec.SSLMode == "" {
		mode = "PREFERRED"
	}

	plain := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?tls=%s",
		spec.User,
		spec.Password,
		spec.Host,
		spec.Port,
		spec.Name,
		mode,
	)

	return []byte(plain)
}

func GetDBSpec(ctx context.Context, store v1.Store, r client.Client) (*DatabaseSpec, error) {
	var dbHost string
	if store.Spec.Database.HostRef.Name != "" {
		hostSecret := new(corev1.Secret)
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: store.Namespace,
			Name:      store.Spec.Database.HostRef.Name,
		}, hostSecret); err != nil {
			return nil, err
		}
		dbHost = string(hostSecret.Data[store.Spec.Database.HostRef.Key])
	} else {
		dbHost = store.Spec.Database.Host
	}

	dbSecret := new(corev1.Secret)
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: store.Namespace,
		Name:      store.Spec.Database.PasswordSecretRef.Name,
	}, dbSecret); err != nil {
		return nil, err
	}

	var password []byte
	var ok bool
	if password, ok = dbSecret.Data[store.Spec.Database.PasswordSecretRef.Key]; !ok {
		return nil, fmt.Errorf("password key %s not found in secret %s", store.Spec.Database.PasswordSecretRef.Key, store.Spec.Database.PasswordSecretRef.Name)
	}

	return &DatabaseSpec{
		Host:     dbHost,
		Password: password,
		User:     store.Spec.Database.User,
		Port:     store.Spec.Database.Port,
		Name:     store.Spec.Database.Name,
		Version:  store.Spec.Database.Version,
		SSLMode:  store.Spec.Database.SSLMode,
		Options:  store.Spec.Database.Options,
	}, nil
}

func GetMysqlShell(ctx context.Context, spec DatabaseSpec) *exec.Cmd {
	return exec.CommandContext(ctx,
		"mysqlsh",
		"--mysql",
		"--schema", "shopware",
		"-h"+spec.Host,
		"-u"+spec.User,
		"-p"+string(spec.Password),
		"--js",
	)
}

func TestSQLConnection(ctx context.Context, spec *DatabaseSpec) error {
	url := GenerateDatabaseURLForGo(spec)
	db, err := sql.Open("mysql", string(url))
	if err != nil {
		return err
	}
	//nolint:errcheck
	defer db.Close()
	err = db.PingContext(ctx)

	if mysqlErr, ok := err.(*mysql.MySQLError); ok {
		// Error 1049 (42000): Unknown database
		if mysqlErr.Number == 1049 {
			return nil
		}
	} else {
		return err
	}

	return nil
}
