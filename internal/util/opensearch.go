package util

import (
	"fmt"
	"net/url"

	v1 "github.com/shopware/shopware-operator/api/v1"
)

func GenerateOpensearchURLForShopware(os *v1.OpensearchSpec, p []byte) []byte {
	urlP := url.QueryEscape(string(p))

	plain := fmt.Sprintf(
		"%s://%s:%s@%s:%d",
		os.Schema,
		os.Username,
		urlP,
		os.Host,
		os.Port,
	)
	return []byte(plain)
}
