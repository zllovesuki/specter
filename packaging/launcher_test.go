package packaging

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLauncherEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("packaged launchers require a POSIX shell")
	}

	for _, service := range []string{"server", "dns", "client"} {
		t.Run(service, func(t *testing.T) {
			template, err := os.ReadFile(filepath.Join("templates", service+"-launch.sh"))
			if err != nil {
				t.Fatal(err)
			}
			for _, test := range []struct {
				name      string
				contents  string
				inherited string
				override  bool
				want      string
			}{
				{name: "bare assignment", contents: "KV_PROVIDER=sqlite\n", want: "sqlite"},
				{name: "exported assignment", contents: "export KV_PROVIDER=sqlite\n", override: true, want: "sqlite"},
				{name: "sourced shell syntax", contents: "SYNTHETIC_KV='sql'\nKV_PROVIDER=\"${SYNTHETIC_KV}ite\" # shell expansion\n", override: true, want: "sqlite"},
				{name: "override inherited value", contents: "KV_PROVIDER=sqlite\n", inherited: "aof", want: "sqlite"},
				{name: "no environment file", inherited: "sqlite", override: true, want: "sqlite"},
				{name: "unset provider", want: "unset"},
			} {
				t.Run(test.name, func(t *testing.T) {
					root := t.TempDir()
					configDir := filepath.Join(root, "specter")
					if err := os.Mkdir(configDir, 0700); err != nil {
						t.Fatal(err)
					}
					write := func(path, contents string, mode os.FileMode) {
						t.Helper()
						if err := os.WriteFile(path, []byte(contents), mode); err != nil {
							t.Fatal(err)
						}
					}
					binDir := filepath.Join(root, "bin")
					if err := os.Mkdir(binDir, 0700); err != nil {
						t.Fatal(err)
					}
					// The stub prints only synthetic values and argv, never the process environment.
					write(filepath.Join(binDir, "specter"), "#!/bin/sh\nprintf '%s\\n' \"${KV_PROVIDER-unset}\" \"$@\"\n", 0700)
					launcher := filepath.Join(root, "launch")
					write(launcher, strings.NewReplacer("__BINDIR__", binDir, "__SYSCONFDIR__", root).Replace(string(template)), 0700)
					config := filepath.Join(root, "client config.yaml")
					write(config, "# synthetic client config\n", 0600)

					command := exec.Command("/bin/sh", launcher)
					command.Env = []string{
						"SPECTER_GLOBAL_ARGS=--verbose --sentry synthetic",
						"SPECTER_SERVER_ARGS=--data-dir /synthetic/data --apex example.test",
						"SPECTER_DNS_ARGS=--rpc unix:///synthetic/rpc.sock",
						"SPECTER_CLIENT_CONFIG=" + config,
						"SPECTER_CLIENT_ARGS=--listen 127.0.0.1:9999",
					}
					if test.inherited != "" {
						command.Env = append(command.Env, "KV_PROVIDER="+test.inherited)
					}
					envFile := filepath.Join(configDir, service+".env")
					if test.override {
						envFile = filepath.Join(root, "override environment.env")
						command.Env = append(command.Env, "SPECTER_"+strings.ToUpper(service)+"_ENV_FILE="+envFile)
					}
					if test.contents != "" {
						write(envFile, test.contents, 0600)
					}

					args := []string{test.want, "--verbose", "--sentry", "synthetic", service}
					switch service {
					case "server":
						args = append(args, "--data-dir", "/synthetic/data", "--apex", "example.test")
					case "dns":
						args = append(args, "--rpc", "unix:///synthetic/rpc.sock")
					case "client":
						args = append(args, "tunnel", "--config", config, "--listen", "127.0.0.1:9999")
					}
					output, err := command.CombinedOutput()
					if err != nil {
						t.Fatalf("launcher: %v\n%s", err, output)
					}
					if want := strings.Join(args, "\n") + "\n"; string(output) != want {
						t.Fatalf("launcher output = %q, want %q", output, want)
					}
				})
			}
		})
	}
}
