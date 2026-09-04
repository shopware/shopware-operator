package util

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"fmt"
	"net"
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

	plain := fmt.Sprintf(
		"mysql://%s:%s@%s:%d/%s?serverVersion=%s%s",
		spec.User,
		urlP,
		spec.Host,
		spec.Port,
		spec.Name,
		spec.Version,
		options,
	)
	return []byte(plain)
}

func GenerateDatabaseURLForGo(spec *DatabaseSpec) []byte {
	plain := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s",
		spec.User,
		spec.Password,
		spec.Host,
		spec.Port,
		spec.Name,
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

	spec := &DatabaseSpec{
		Host:                           dbHost,
		Password:                       password,
		User:                           store.Spec.Database.User,
		Port:                           store.Spec.Database.Port,
		Name:                           store.Spec.Database.Name,
		Version:                        store.Spec.Database.Version,
		Options:                        store.Spec.Database.Options,
		TLSClientCertificate:           store.Spec.Database.TLS.ClientCertificate,
		TLSDontVerifyServerCertificate: store.Spec.Database.TLS.DontVerifyServerCertificate,
	}

	tlsRequired := store.Spec.Database.TLS.SecretName != "" || store.Spec.Database.RequiresTLSSecret()
	if !tlsRequired {
		return spec, nil
	}
	if store.Spec.Database.TLS.SecretName == "" {
		return nil, fmt.Errorf("database tls secretName is required when sslMode is REQUIRED")
	}

	tlsSecret := new(corev1.Secret)
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: store.Namespace,
		Name:      store.Spec.Database.TLS.SecretName,
	}, tlsSecret); err != nil {
		return nil, fmt.Errorf("get database tls secret: %w", err)
	}
	if spec.TLSCA, ok = tlsSecret.Data["ca.crt"]; !ok || len(spec.TLSCA) == 0 {
		return nil, fmt.Errorf("ca.crt key not found in database tls secret %s", store.Spec.Database.TLS.SecretName)
	}
	if spec.TLSClientCertificate {
		if spec.TLSCert, ok = tlsSecret.Data["tls.crt"]; !ok || len(spec.TLSCert) == 0 {
			return nil, fmt.Errorf("tls.crt key not found in database tls secret %s", store.Spec.Database.TLS.SecretName)
		}
		if spec.TLSKey, ok = tlsSecret.Data["tls.key"]; !ok || len(spec.TLSKey) == 0 {
			return nil, fmt.Errorf("tls.key key not found in database tls secret %s", store.Spec.Database.TLS.SecretName)
		}
	}

	return spec, nil
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
	port := spec.Port
	if port == 0 {
		port = 3306
	}
	config := mysql.NewConfig()
	config.User = spec.User
	config.Passwd = string(spec.Password)
	config.Net = "tcp"
	config.Addr = net.JoinHostPort(spec.Host, fmt.Sprintf("%d", port))
	config.DBName = spec.Name

	if len(spec.TLSCA) > 0 {
		tlsConfig, err := databaseTLSConfig(spec)
		if err != nil {
			return err
		}
		config.TLS = tlsConfig
	}

	connector, err := mysql.NewConnector(config)
	if err != nil {
		return err
	}
	db := sql.OpenDB(connector)
	//nolint:errcheck
	defer db.Close()
	err = db.PingContext(ctx)
	if err != nil {
		// Error 1049 (42000): Unknown database
		if mysqlErr, ok := err.(*mysql.MySQLError); ok && mysqlErr.Number == 1049 {
			return nil
		}
		return err
	}

	return nil
}

func databaseTLSConfig(spec *DatabaseSpec) (*tls.Config, error) {
	roots := x509.NewCertPool()
	if ok := roots.AppendCertsFromPEM(spec.TLSCA); !ok {
		return nil, fmt.Errorf("parse database TLS CA certificate")
	}

	config := &tls.Config{
		RootCAs:            roots,
		InsecureSkipVerify: spec.TLSDontVerifyServerCertificate, //nolint:gosec // explicitly configured by the Store
	}
	if !spec.TLSClientCertificate {
		return config, nil
	}

	certificate, err := tls.X509KeyPair(spec.TLSCert, spec.TLSKey)
	if err != nil {
		return nil, fmt.Errorf("parse database TLS client certificate: %w", err)
	}
	config.Certificates = []tls.Certificate{certificate}
	return config, nil
}
