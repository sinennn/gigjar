package logging

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestSensitiveAttributesAreRedacted(t *testing.T) {
	var buffer bytes.Buffer
	logger := New(&buffer, "info", "json")

	logger.Info("signing",
		"password", "hunter2",
		"seed_phrase", "abandon abandon",
		"Authorization", "Bearer abc",
		"tron_api_key", "xyz",
		"address", "TJarAddress",
	)

	var record map[string]any
	if err := json.Unmarshal(buffer.Bytes(), &record); err != nil {
		t.Fatalf("log line is not JSON: %v", err)
	}
	for _, key := range []string{"password", "seed_phrase", "Authorization", "tron_api_key"} {
		if record[key] != redactedValue {
			t.Errorf("%s = %v, want redacted", key, record[key])
		}
	}
	if record["address"] != "TJarAddress" {
		t.Errorf("address = %v, want unredacted", record["address"])
	}
	for _, secret := range []string{"hunter2", "abandon", "Bearer abc", "xyz"} {
		if strings.Contains(buffer.String(), secret) {
			t.Errorf("log output leaked %q", secret)
		}
	}
}

func TestRedactionAppliesInsideGroups(t *testing.T) {
	var buffer bytes.Buffer
	logger := New(&buffer, "info", "json")

	logger.WithGroup("signer").Info("ready", "private_key", "0xdeadbeef")

	if strings.Contains(buffer.String(), "deadbeef") {
		t.Errorf("grouped secret leaked: %s", buffer.String())
	}
}

func TestLevelFiltering(t *testing.T) {
	var buffer bytes.Buffer
	logger := New(&buffer, "warn", "text")

	logger.Info("hidden")
	logger.Warn("shown")

	output := buffer.String()
	if strings.Contains(output, "hidden") || !strings.Contains(output, "shown") {
		t.Errorf("unexpected output: %q", output)
	}
}
