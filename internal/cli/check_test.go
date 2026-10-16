package cli

import (
	"strings"
	"testing"

	"github.com/stackmade/driftr/internal/resolver"
)

func TestWinnerIndex(t *testing.T) {
	pins := []resolver.Pin{
		{Tool: "node", Version: "lts/mithril", Source: resolver.SourceNvmrc},
		{Tool: "node", Version: "20.11.0", Source: resolver.SourceNodeVersion},
	}
	if got := winnerIndex(pins); got != 1 {
		t.Errorf("winnerIndex() = %d, want 1: the resolver skips the unknown codename", got)
	}
	if got := winnerIndex(pins[:1]); got != -1 {
		t.Errorf("winnerIndex() = %d, want -1 when every source is skipped", got)
	}
	pins[0].Source = resolver.SourceProject
	if got := winnerIndex(pins); got != 0 {
		t.Errorf("winnerIndex() = %d, want 0: .driftr.toml is never skipped", got)
	}
}

func TestCheckToolPins(t *testing.T) {
	const cwd = "/proj"
	pin := func(ver string, src resolver.Source) resolver.Pin {
		return resolver.Pin{Tool: "node", Version: ver, Source: src, Dir: cwd}
	}
	toml := func(v string) resolver.Pin { return pin(v, resolver.SourceProject) }
	nvmrc := func(v string) resolver.Pin { return pin(v, resolver.SourceNvmrc) }
	engines := func(v string) resolver.Pin { return pin(v, resolver.SourceEnginesNode) }

	tests := []struct {
		name      string
		pins      []resolver.Pin
		installed map[string]string // spec -> version ResolvePin returns
		want      []string          // substrings, one per expected problem
	}{
		{name: "single source", pins: []resolver.Pin{toml("22.14.0")}},
		{name: "same version", pins: []resolver.Pin{toml("22.14.0"), nvmrc("22.14.0")}},
		{name: "partial agrees with full", pins: []resolver.Pin{toml("22"), nvmrc("22.14.0")}},
		{name: "full agrees with partial", pins: []resolver.Pin{toml("22.14.0"), nvmrc("v22")}},
		{name: "lts agrees with an LTS line", pins: []resolver.Pin{nvmrc("lts/*"), pin("22", resolver.SourceNodeVersion)}},
		{name: "codename agrees with its line", pins: []resolver.Pin{toml("22.14.0"), nvmrc("lts/jod")}},
		{
			name: "different majors",
			pins: []resolver.Pin{toml("22.14.0"), nvmrc("20")},
			want: []string{".nvmrc says 20, but .driftr.toml says 22.14.0 and wins"},
		},
		{
			name: "different minors",
			pins: []resolver.Pin{toml("22.14"), nvmrc("22.15.0")},
			want: []string{".nvmrc says 22.15.0"},
		},
		{
			name: "codename for another line",
			pins: []resolver.Pin{toml("22"), nvmrc("lts/iron")},
			want: []string{".nvmrc says lts/iron"},
		},
		{
			name: "engines satisfied by the pin",
			pins: []resolver.Pin{toml("24.1.0"), engines(">=24")},
		},
		{
			name: "winner outside engines",
			pins: []resolver.Pin{toml("22.14.0"), engines(">=24")},
			want: []string{`node 22.14.0 from .driftr.toml is outside package.json (engines.node) ">=24"`},
		},
		{
			name: "uninstalled partial pin that could satisfy engines",
			pins: []resolver.Pin{toml("22"), engines(">=22.5")},
		},
		{
			name:      "installed version of a partial pin is outside engines",
			pins:      []resolver.Pin{toml("22"), engines(">=22.5")},
			installed: map[string]string{"22": "22.1.0"},
			want:      []string{"resolves to 22.1.0, which is outside"},
		},
		{
			name:      "installed version of a partial pin satisfies engines",
			pins:      []resolver.Pin{toml("22"), engines(">=22.5")},
			installed: map[string]string{"22": "22.9.0"},
		},
		{
			name: "a parent pin beats engines and falls outside it",
			pins: []resolver.Pin{{Tool: "node", Version: "20", Source: resolver.SourceNvmrc, Dir: "/"}, engines(">=24")},
			want: []string{`node 20 from ../.nvmrc is outside package.json (engines.node) ">=24"`},
		},
		{
			name:      "engines wins, the version it runs is outside a parent's engines",
			pins:      []resolver.Pin{engines(">=18"), {Tool: "node", Version: "<20", Source: resolver.SourceEnginesNode, Dir: "/"}},
			installed: map[string]string{">=18": "24.1.0"},
			want:      []string{`node >=18 from package.json (engines.node) resolves to 24.1.0, which is outside ../package.json (engines.node) "<20"`},
		},
		{
			name:      "engines wins, the version it runs satisfies a parent's engines",
			pins:      []resolver.Pin{engines(">=18"), {Tool: "node", Version: "<20", Source: resolver.SourceEnginesNode, Dir: "/"}},
			installed: map[string]string{">=18": "19.0.0"},
		},
		{
			name: "engines wins with nothing installed, overlapping ranges agree",
			pins: []resolver.Pin{engines(">=18"), {Tool: "node", Version: "<20", Source: resolver.SourceEnginesNode, Dir: "/"}},
		},
		{
			name: "both a conflict and an engines violation",
			pins: []resolver.Pin{toml("22.14.0"), nvmrc("20"), engines(">=24")},
			want: []string{".nvmrc says 20", "is outside package.json (engines.node)"},
		},
		{
			name: "invalid version",
			pins: []resolver.Pin{toml("22.14.0"), nvmrc("twenty")},
			want: []string{`.nvmrc: cannot read "twenty"`},
		},
		{
			name: "invalid engines range",
			pins: []resolver.Pin{toml("22.14.0"), engines("18 - 20")},
			want: []string{`package.json (engines.node): cannot read "18 - 20"`},
		},
		{
			name: "invalid winner is reported and nothing compared against it",
			pins: []resolver.Pin{toml("abc"), nvmrc("20")},
			want: []string{`.driftr.toml: cannot read "abc"`},
		},
		{
			name: "unknown codename",
			pins: []resolver.Pin{nvmrc("lts/nosuchname"), toml("20")},
			want: []string{"an LTS codename this driftr does not know"},
		},
		{
			name: "the resolver skips an unknown codename, the next source wins",
			pins: []resolver.Pin{nvmrc("lts/mithril"), pin("20.11.0", resolver.SourceNodeVersion), engines(">=24")},
			want: []string{
				"an LTS codename this driftr does not know",
				`node 20.11.0 from .node-version is outside package.json (engines.node) ">=24"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolve := func(p resolver.Pin) (string, bool, error) {
				v, ok := tt.installed[p.Version]
				return v, ok, nil
			}
			got := checkToolPins(cwd, tt.pins, resolve)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d problems %+v, want %d", len(got), got, len(tt.want))
			}
			for i, w := range tt.want {
				if !strings.Contains(got[i].msg, w) {
					t.Errorf("problem %d = %q, want it to contain %q", i, got[i].msg, w)
				}
				if got[i].hint == "" {
					t.Errorf("problem %d has no hint", i)
				}
			}
		})
	}
}
