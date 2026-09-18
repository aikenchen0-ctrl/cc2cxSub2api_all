package app

import (
	"encoding/json"
	"testing"
)

func TestEmptyModelCatalogRetainsActiveCollection(t *testing.T) {
	for _, test := range []struct {
		catalog ModelCatalogResponse
		want    string
	}{
		{ModelCatalogResponse{Source: ModelCatalogSourceSystem, Channels: []PublicChannelCatalog{}}, `{"source":"system","channels":[]}`},
		{ModelCatalogResponse{Source: ModelCatalogSourceFrontend, Models: []PublicLogicalModel{}}, `{"source":"frontend","models":[]}`},
	} {
		body, err := json.Marshal(test.catalog)
		if err != nil || string(body) != test.want {
			t.Fatalf("empty catalog = %s, error = %v", body, err)
		}
	}
}
