package cli

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestJSONContract(t *testing.T) {
	for _, tt := range []struct {
		args []string
		exit int
		code string
	}{{[]string{"version"}, 0, ""}, {[]string{"exec"}, 2, "invalid"}, {[]string{"exec", "--plan", "/no-such-malta-plan", "--out", "/tmp/unused"}, 3, "execution"}, {[]string{"--format", "text", "version"}, 2, "invalid"}} {
		var out, errOut bytes.Buffer
		if code := Execute(tt.args, &out, &errOut); code != tt.exit {
			t.Fatalf("%v: exit %d, %s", tt.args, code, out.String())
		}
		var e Envelope
		if err := json.Unmarshal(out.Bytes(), &e); err != nil {
			t.Fatal(err, out.String())
		}
		if e.SchemaVersion != "1.0" {
			t.Fatal(e)
		}
		if tt.code != "" && (e.Error == nil || e.Error.Code != tt.code) {
			t.Fatal(e)
		}
	}
}
