package shell

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFishSearchProtocol(t *testing.T) {
	if _, err := exec.LookPath("fish"); err != nil {
		t.Skip("fish not available")
	}

	for _, test := range []struct {
		name   string
		result string
		code   string
		want   string
	}{
		{"select multiline", "select\nfirst\n\nlast", "0", "replace=<first\n\nlast>\nrepaint\n"},
		{"execute", "execute\necho ready", "0", "replace=<echo ready>\nexecute\nrepaint\n"},
		{"failed partial result", "execute\necho unsafe", "2", "repaint\n"},
		{"cancel", "", "1", "repaint\n"},
	} {
		t.Run(test.name, func(t *testing.T) { runFishSearchProtocol(t, test.result, test.code, test.want) })
	}
}

func runFishSearchProtocol(t *testing.T, result, code, want string) {
	t.Helper()
	dir := t.TempDir()
	fake := filepath.Join(dir, "zgod")
	// A search must pass an empty query as an explicit argument.
	const program = "#!/bin/sh\n[ \"$5\" = --query ] && [ \"$#\" = 6 ] && [ -z \"$6\" ] || exit 99\nprintf '%s' \"$ZGOD_TEST_RESULT\"\nexit \"$ZGOD_TEST_CODE\"\n"
	if err := os.WriteFile(fake, []byte(program), 0o755); err != nil {
		t.Fatal(err)
	}

	script, err := InitScript(Fish, InitOptions{BinPath: fake})
	if err != nil {
		t.Fatal(err)
	}
	// The editor API is stubbed to inspect replacement and execute requests.
	script = strings.ReplaceAll(script, "</dev/tty", "</dev/null") + `
functions -e __zgod_preexec __zgod_postexec
function commandline
 switch "$argv[1]"
 case -r
  printf 'replace=<%s>\n' "$argv[2]"
 case -f
  printf '%s\n' "$argv[2]"
 end # switch action
end # commandline stub
__zgod_search
`

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	command := exec.CommandContext(ctx, "fish", "-c", script)

	command.Env = append(os.Environ(), "HOME="+dir, "ZGOD_TEST_RESULT="+result, "ZGOD_TEST_CODE="+code)

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("fish search: %v: %s", err, output)
	}

	if string(output) != want {
		t.Fatalf("output=%q, want %q", output, want)
	}
}
