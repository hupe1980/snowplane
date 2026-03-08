package snowplane_test

import (
	"fmt"
	"testing"

	"github.com/gobuffalo/flect"
)

func TestFlectPlurals(t *testing.T) {
	// Note: flect.Pluralize does NOT correctly handle "aws" and "gcs" suffixes.
	// flect returns the input unchanged, but controller-gen produces
	// "storageintegrationawses" and "storageintegrationgcses" in the CRD YAMLs.
	// The RBAC coverage tests read plurals from CRD files (source of truth)
	// rather than relying on flect.
	words := []string{
		"storageintegrationaws",
		"storageintegrationgcs",
		"storageintegrationazure",
		"internalstage",
		"externalstage",
		"apiauthenticationintegrationwithclientcredentials",
		"secretwithclientcredentials",
	}
	for _, w := range words {
		fmt.Printf("%-55s -> %s\n", w, flect.Pluralize(w))
	}
}
