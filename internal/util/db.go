package util

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

	"github.com/go-sql-driver/mysql"
	v1 "github.com/shopware/shopware-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GenerateDatabaseURLForShopware(db *v1.DatabaseSpec, dbHost string, p []byte) []byte {
	urlP := url.QueryEscape(string(p))

	var options string
	if db.Options != "" {
		options = "&" + db.Options
	}

	mode := db.SSLMode
	if db.SSLMode == "" {
		mode = "PREFERRED"
	}

	plain := fmt.Sprintf(
		"mysql://%s:%s@%s:%d/%s?serverVersion=%s&sslMode=%s%s",
		db.User,
		urlP,
		dbHost,
		db.Port,
		db.Name,
		db.Version,
		mode,
		options,
	)
	return []byte(plain)
}

func GenerateDatabaseURLForGo(db *v1.DatabaseSpec, dbHost string, p []byte) []byte {
	mode := db.SSLMode
	if db.SSLMode == "" {
		mode = "PREFERRED"
	}

	plain := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?tls=%s",
		db.User,
		p,
		dbHost,
		db.Port,
		db.Name,
		mode,
	)

	return []byte(plain)
}

func GetDBHost(ctx context.Context, store v1.Store, r client.Client) (string, error) {
	var dbHost string
	if store.Spec.Database.HostRef.Name != "" {
		hostSecret := new(corev1.Secret)
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: store.Namespace,
			Name:      store.Spec.Database.HostRef.Name,
		}, hostSecret); err != nil {
			if k8serrors.IsNotFound(err) {
				return "", fmt.Errorf("can't find db secret, but host variable is empty: %w", err)
			}
			return "", fmt.Errorf("error in getting secret: %w", err)
		}
		dbHost = string(hostSecret.Data[store.Spec.Database.HostRef.Key])
	} else {
		dbHost = store.Spec.Database.Host
	}
	return dbHost, nil
}

func TestSQLConnection(ctx context.Context, database *v1.DatabaseSpec, host string, p []byte) error {
	url := GenerateDatabaseURLForGo(database, host, p)
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
