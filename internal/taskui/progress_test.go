package taskui

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestProgressFrameCarriesMeasurementsWithoutEmbeddingTerminalLayout(t *testing.T) {
	status := Progress{Label: "synthetic.mp4", Detail: "30 MiB / 100 MiB", Summary: "ETA 10s", Percent: .3, Determinate: true, CompletedFiles: 2, TotalFiles: 8}
	var out bytes.Buffer
	WriteStatus(&out, status)
	prefix := "\x1b]777;" + ProgressPrefix
	if !strings.HasPrefix(out.String(), prefix) || !strings.HasSuffix(out.String(), "\a") {
		t.Fatal("missing framed worker progress")
	}
	var decoded Progress
	if err := json.Unmarshal([]byte(strings.TrimSuffix(strings.TrimPrefix(out.String(), prefix), "\a")), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded != status {
		t.Fatalf("lost a transfer measurement: %#v", decoded)
	}
	out.Reset()
	WriteStatus(&out, Progress{Label: strings.Repeat("x", 8192)})
	if out.Len() != 0 {
		t.Fatal("an oversized frame should not enter the child terminal stream")
	}
}
