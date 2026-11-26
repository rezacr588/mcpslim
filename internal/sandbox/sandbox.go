// Package sandbox builds least-privilege OS wrappers for upstream MCP
// servers: Seatbelt on macOS, bubblewrap on Linux.
package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Policy describes what an upstream server may touch.
type Policy struct {
	Enabled     bool
	Workspace   string
	DenyNetwork bool
	Home        string
}

// DefaultDenyPaths are credential locations no MCP server needs.
func DefaultDenyPaths(home string) []string {
	if home == "" {
		return nil
	}
	return []string{
		filepath.Join(home, ".ssh"),
		filepath.Join(home, ".aws"),
		filepath.Join(home, ".gnupg"),
		filepath.Join(home, ".netrc"),
		filepath.Join(home, ".config", "gcloud"),
	}
}

// Wrap returns argv prefixed with the platform sandbox, or argv unchanged when
// sandboxing is unavailable. The second return value is a human-readable note
// explaining a fallback ("" when the sandbox was applied).
func Wrap(argv []string, p Policy) ([]string, string) {
	if !p.Enabled || len(argv) == 0 {
		return argv, ""
	}
	if p.Workspace == "" {
		p.Workspace, _ = os.Getwd()
	}
	if p.Home == "" {
		p.Home, _ = os.UserHomeDir()
	}
	switch runtime.GOOS {
	case "darwin":
		exe := "/usr/bin/sandbox-exec"
		if _, err := os.Stat(exe); err != nil {
			return argv, "sandbox-exec not found; running without a sandbox"
		}
		out := []string{exe, "-p", Profile(p)}
		return append(out, argv...), ""
	case "linux":
		exe, err := exec.LookPath("bwrap")
		if err != nil {
			return argv, "bwrap not found; running without a sandbox"
		}
		out := []string{exe,
			"--ro-bind", "/", "/",
			"--dev", "/dev",
			"--proc", "/proc",
			"--bind", p.Workspace, p.Workspace,
		}
		for _, path := range DefaultDenyPaths(p.Home) {
			if _, err := os.Stat(path); err == nil {
				out = append(out, "--tmpfs", path)
			}
		}
		if p.DenyNetwork {
			out = append(out, "--unshare-net")
		}
		return append(out, argv...), ""
	default:
		return argv, "sandboxing is not implemented on " + runtime.GOOS
	}
}

// Profile renders a Seatbelt profile. The default policy is permissive except
// for credential paths, workspace .env files and, optionally, the network.
func Profile(p Policy) string {
	var b strings.Builder
	b.WriteString("(version 1)\n")
	b.WriteString("(allow default)\n")
	for _, path := range DefaultDenyPaths(resolve(p.Home)) {
		if path == "" {
			continue
		}
		b.WriteString("(deny file-read* file-write* (subpath ")
		b.WriteString(quote(path))
		b.WriteString("))\n")
	}
	if p.Workspace != "" {
		b.WriteString("(deny file-read* (regex ")
		b.WriteString(quote(`^` + regexEscape(resolve(p.Workspace)+string(os.PathSeparator)) + `\.env(\..*)?$`))
		b.WriteString("))\n")
	}
	if p.DenyNetwork {
		b.WriteString("(deny network*)\n")
	}
	return b.String()
}

// resolve expands symlinks because Seatbelt matches the resolved path. On
// macOS /tmp and /var are symlinks, so an unresolved deny silently does nothing.
func resolve(path string) string {
	if path == "" {
		return ""
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	return path
}

func quote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func regexEscape(s string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`.`, `\.`,
		`+`, `\+`,
		`?`, `\?`,
		`(`, `\(`,
		`)`, `\)`,
		`[`, `\[`,
		`]`, `\]`,
		`{`, `\{`,
		`}`, `\}`,
		`^`, `\^`,
		`$`, `\$`,
		`|`, `\|`,
	)
	return replacer.Replace(s)
}
