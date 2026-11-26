package lockfile

import (
	"path/filepath"
	"testing"

	"github.com/rezacr588/mcpslim/internal/mcp"
)

func tool(desc, schema string) mcp.Tool {
	return mcp.Tool{Name: "read_file", Description: desc, InputSchema: []byte(schema)}
}

func TestCanonicalIgnoresFormatting(t *testing.T) {
	a := tool("Read a file\nfrom disk", `{"type":"object","properties":{"b":{"type":"string"},"a":{"type":"string"}}}`)
	b := tool("Read a file   from disk", `{"properties":{"a":{"type":"string"},"b":{"type":"string"}},"type":"object"}`)
	ha, err := Hash(a)
	if err != nil {
		t.Fatal(err)
	}
	hb, err := Hash(b)
	if err != nil {
		t.Fatal(err)
	}
	if ha != hb {
		t.Fatalf("hashes differ: %s != %s", ha, hb)
	}
}

func TestCheckPinsThenDetectsDrift(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcpslim.lock")
	l, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	tools := []mcp.Tool{tool("Read a file", `{"type":"object"}`)}

	drift, changed, err := l.Check("fs", tools)
	if err != nil || len(drift) != 0 || !changed {
		t.Fatalf("first check: drift=%v changed=%v err=%v", drift, changed, err)
	}
	drift, changed, err = l.Check("fs", tools)
	if err != nil || len(drift) != 0 || changed {
		t.Fatalf("stable check: drift=%v changed=%v err=%v", drift, changed, err)
	}

	changedTool := tool("Read a file and print it", `{"type":"object"}`)
	drift, changed, err = l.Check("fs", []mcp.Tool{changedTool})
	if err != nil || len(drift) != 1 || !changed {
		t.Fatalf("drift check: drift=%v changed=%v err=%v", drift, changed, err)
	}
	if pending := l.Pending(); len(pending) != 1 {
		t.Fatalf("pending = %v", pending)
	}
	if n := l.Approve("fs", "read_file"); n != 1 {
		t.Fatalf("approved %d, want 1", n)
	}
	if len(l.Pending()) != 0 {
		t.Fatal("pending not cleared")
	}
	drift, _, err = l.Check("fs", []mcp.Tool{changedTool})
	if err != nil || len(drift) != 0 {
		t.Fatalf("after approve: drift=%v err=%v", drift, err)
	}
}

func TestLoadMissingFile(t *testing.T) {
	l, err := Load(filepath.Join(t.TempDir(), "nope.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if l.Version != Version || len(l.Servers) != 0 {
		t.Fatalf("unexpected empty lock: %+v", l)
	}
}
