package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProfileDeniesCredentialsAndEnv(t *testing.T) {
	p := Policy{Enabled: true, Workspace: "/work/project", Home: "/home/dev"}
	profile := Profile(p)
	for _, want := range []string{
		`(deny file-read* file-write* (subpath "/home/dev/.ssh"))`,
		`(deny file-read* file-write* (subpath "/home/dev/.aws"))`,
		`(deny file-read* file-write* (subpath "/home/dev/.gnupg"))`,
		`/work/project/`,
	} {
		if !strings.Contains(profile, want) {
			t.Errorf("profile missing %q:\n%s", want, profile)
		}
	}
	if strings.Contains(profile, "(deny network*)") {
		t.Error("network denied by default")
	}
	denied := Profile(Policy{Enabled: true, Home: "/home/dev", DenyNetwork: true})
	if !strings.Contains(denied, "(deny network*)") {
		t.Error("DenyNetwork did not render")
	}
}

func TestWrapWithoutSandboxIsIdentity(t *testing.T) {
	argv := []string{"echo", "hi"}
	got, _ := Wrap(argv, Policy{Enabled: false})
	if len(got) != 2 || got[0] != "echo" {
		t.Fatalf("got %v", got)
	}
}

func TestSeatbeltEnforcement(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Seatbelt is macOS only")
	}
	if _, err := os.Stat("/usr/bin/sandbox-exec"); err != nil {
		t.Skip("sandbox-exec not available")
	}
	home := t.TempDir()
	ssh := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(ssh, 0o700); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(ssh, "id_rsa")
	if err := os.WriteFile(secret, []byte("PRIVATE KEY"), 0o600); err != nil {
		t.Fatal(err)
	}
	allowed := filepath.Join(home, "notes.txt")
	if err := os.WriteFile(allowed, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	profile := Profile(Policy{Enabled: true, Workspace: home, Home: home})
	out, err := exec.Command("/usr/bin/sandbox-exec", "-p", profile, "/bin/cat", allowed).CombinedOutput()
	if err != nil {
		t.Fatalf("allowed read failed: %v\n%s\nprofile:\n%s", err, out, profile)
	}
	if err := exec.Command("/usr/bin/sandbox-exec", "-p", profile, "/bin/cat", secret).Run(); err == nil {
		t.Fatal("sandbox allowed reading ~/.ssh/id_rsa")
	}

	envFile := filepath.Join(home, ".env")
	if err := os.WriteFile(envFile, []byte("API_KEY=secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("/usr/bin/sandbox-exec", "-p", profile, "/bin/cat", envFile).Run(); err == nil {
		t.Fatal("sandbox allowed reading workspace .env")
	}
}
