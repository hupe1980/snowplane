package snowflake

import (
	"sort"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// setWebhookHeaders writes sorted WEBHOOK_HEADER_<key> clauses to the builder.
func setWebhookHeaders(b *sqlbuilder.Builder, headers map[string]string) {
	if len(headers) == 0 {
		return
	}

	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, k := range keys {
		v := headers[k]
		b.SetString("WEBHOOK_HEADER_"+k, &v)
	}
}
