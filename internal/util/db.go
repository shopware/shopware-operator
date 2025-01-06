package util

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

	_ "github.com/go-sql-driver/mysql"
	v1 "github.com/shopware/shopware-operator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func GenerateDatabaseURLForShopware(db *v1.DatabaseSpec, dbHost string, p []byte) []byte {
	urlP := url.QueryEscape(string(p))
	plain := fmt.Sprintf(
		"mysql://%s:%s@%s:%d/%s?serverVersion=%s&sslMode=%s",
		db.User,
		urlP,
		dbHost,
		db.Port,
		db.Name,
		db.Version,
		db.SSLMode,
	)
	return []byte(plain)
}

func GenerateDatabaseURLForGo(db *v1.DatabaseSpec, p []byte) []byte {
	plain := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s",
		db.User,
		p,
		db.Host,
		db.Port,
		db.Name,
	)
	return []byte(plain)
}

func TestSQLConnection(ctx context.Context, database *v1.DatabaseSpec, p []byte) error {
	url := GenerateDatabaseURLForGo(database, p)
	log.FromContext(ctx).Info("db url", "url", string(url))
	db, err := sql.Open("mysql", string(url))
	if err != nil {
		return err
	}
	//nolint:errcheck
	defer db.Close()
	return db.PingContext(ctx)
}
