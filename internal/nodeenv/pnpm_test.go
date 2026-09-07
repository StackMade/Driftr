package nodeenv

import (
	"errors"
	"strings"
	"testing"
)

// fakeRunner records invocations and returns scripted output keyed by the joined
// command line. Missing keys return empty output and no error.
type fakeRunner struct {
	calls   []string
	outputs map[string]string
	errs    map[string]error
	missing map[string]bool // names that LookPath should fail for
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{outputs: map[string]string{}, errs: map[string]error{}, missing: map[string]bool{}}
}

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	key := strings.TrimSpace(name + " " + strings.Join(args, " "))
	f.calls = append(f.calls, key)
	return f.outputs[key], f.errs[key]
}

func (f *fakeRunner) LookPath(name string) (string, error) {
	if f.missing[name] {
		return "", errors.New("not found")
	}
	return "/usr/bin/" + name, nil
}

func (f *fakeRunner) called(cmd string) bool {
	for _, c := range f.calls {
		if c == cmd {
			return true
		}
	}
	return false
}

func TestPnpm_Installed(t *testing.T) {
	f := newFakeRunner()
	if !NewPnpm(f).Installed() {
		t.Error("expected Installed=true when pnpm on PATH")
	}
	f.missing["pnpm"] = true
	if NewPnpm(f).Installed() {
		t.Error("expected Installed=false when pnpm missing")
	}
}

func TestPnpm_CorepackAvailable(t *testing.T) {
	f := newFakeRunner()
	if !NewPnpm(f).CorepackAvailable() {
		t.Error("expected CorepackAvailable=true when corepack on PATH")
	}
	f.missing["corepack"] = true
	if NewPnpm(f).CorepackAvailable() {
		t.Error("expected CorepackAvailable=false when corepack missing")
	}
	// pnpm missing must not affect the corepack answer, and vice versa.
	f2 := newFakeRunner()
	f2.missing["pnpm"] = true
	if !NewPnpm(f2).CorepackAvailable() {
		t.Error("CorepackAvailable must not depend on pnpm being present")
	}
}

func TestPnpm_Version(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		err     error
		want    string
		wantErr bool
	}{
		{name: "reports version", output: "9.1.0", want: "9.1.0"},
		{name: "empty output returned verbatim", output: "", want: ""},
		{name: "error propagates", err: errors.New("pnpm exploded"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeRunner()
			f.outputs["pnpm --version"] = tt.output
			if tt.err != nil {
				f.errs["pnpm --version"] = tt.err
			}
			got, err := NewPnpm(f).Version()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Version() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Version() = %q, want %q", got, tt.want)
			}
			if !f.called("pnpm --version") {
				t.Errorf("expected call %q, got %v", "pnpm --version", f.calls)
			}
		})
	}
}

func TestPnpm_InstallCommand(t *testing.T) {
	f := newFakeRunner()
	f.outputs["pnpm install"] = "Done in 1.2s"
	got, err := NewPnpm(f).Install()
	if err != nil {
		t.Fatal(err)
	}
	if got != "Done in 1.2s" {
		t.Errorf("Install() = %q, want %q", got, "Done in 1.2s")
	}
	if len(f.calls) != 1 || f.calls[0] != "pnpm install" {
		t.Errorf("expected exactly one call %q, got %v", "pnpm install", f.calls)
	}
}

func TestPnpm_InstallErrorPropagates(t *testing.T) {
	f := newFakeRunner()
	f.errs["pnpm install"] = errors.New("ERR_PNPM_NO_LOCKFILE")
	out, err := NewPnpm(f).Install()
	if err == nil {
		t.Fatal("expected error to propagate")
	}
	if out != "" {
		t.Errorf("Install() output on error = %q, want empty", out)
	}
}

func TestPnpm_ConfigSetCommand(t *testing.T) {
	f := newFakeRunner()
	if err := NewPnpm(f).ConfigSet(StoreDirKey, "/store"); err != nil {
		t.Fatal(err)
	}
	want := "pnpm config set store-dir /store"
	if !f.called(want) {
		t.Errorf("expected call %q, got %v", want, f.calls)
	}
}

func TestPnpm_ConfigGetReturnsOutput(t *testing.T) {
	f := newFakeRunner()
	f.outputs["pnpm config get enable-global-virtual-store"] = "true"
	got, err := NewPnpm(f).ConfigGet(GlobalVirtualStoreKey)
	if err != nil {
		t.Fatal(err)
	}
	if got != "true" {
		t.Errorf("ConfigGet = %q, want %q", got, "true")
	}
}

func TestPnpm_CorepackEnableCommand(t *testing.T) {
	f := newFakeRunner()
	if err := NewPnpm(f).CorepackEnable(); err != nil {
		t.Fatal(err)
	}
	if !f.called("corepack enable") {
		t.Errorf("expected 'corepack enable', got %v", f.calls)
	}
}

func TestPnpm_StoreCommands(t *testing.T) {
	f := newFakeRunner()
	f.outputs["pnpm store path"] = "/store/pnpm"
	p := NewPnpm(f)
	if got, _ := p.StorePath(); got != "/store/pnpm" {
		t.Errorf("StorePath = %q", got)
	}
	if _, err := p.StorePrune(); err != nil {
		t.Fatal(err)
	}
	if !f.called("pnpm store prune") {
		t.Errorf("expected 'pnpm store prune', got %v", f.calls)
	}
}

func TestPnpm_RunErrorPropagates(t *testing.T) {
	f := newFakeRunner()
	f.errs["pnpm config set store-dir /store"] = errors.New("boom")
	if err := NewPnpm(f).ConfigSet(StoreDirKey, "/store"); err == nil {
		t.Error("expected error to propagate")
	}
}
