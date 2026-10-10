package cosermetadata

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPublicPreviewSerializesEmptyAccountsAsArray(t *testing.T) {
	data, err := json.Marshal(publicPreview(PreparedPreview{Token: "token"}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"accounts":[]`) || strings.Contains(string(data), `"accounts":null`) {
		t.Fatalf("empty preview accounts JSON = %s", data)
	}
}
