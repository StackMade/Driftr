// Package fixture serves a minimal fake of every upstream driftr downloads
// from: the nodejs.org distribution index, the npm registry, and bun's GitHub
// releases, plus driftr's own release feed for self-update. Tests point driftr
// at it through DRIFTR_NODE_MIRROR, DRIFTR_NPM_REGISTRY, DRIFTR_BUN_RELEASES,
// DRIFTR_BUN_MIRROR, DRIFTR_UPDATE_API and DRIFTR_UPDATE_MIRROR, so the suite
// never touches the real network.
//
// Routes:
//
//	/index.json                                     node release index
//	/v<ver>/SHASUMS256.txt                          node checksums, all platforms
//	/v<ver>/node-v<ver>-<os>-<arch>.tar.gz          node tarball
//	/registry/<pkg>                                 npm package metadata
//	/registry/<pkg>/<version>                       npm version metadata
//	/registry/<pkg>/-/<pkg>-<version>.tgz           npm tarball
//	/bun/releases                                   bun release index
//	/bun/download/bun-v<ver>/SHASUMS256.txt         bun checksums
//	/bun/download/bun-v<ver>/bun-<os>-<arch>.zip    bun asset
//	/update/api/releases/latest                     driftr release index
//	/update/download/v<ver>/checksums.txt           driftr release checksums
//	/update/download/v<ver>/driftr_<ver>_<os>_<arch>.tar.gz  driftr release archive
//
// The archives hold shell scripts rather than real binaries. Each one prints a
// version string and echoes the arguments it received, which is enough to
// assert which file the shim actually exec'd.
package fixture

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// Versions served by the fixture. They sit far above anything real so a test
// asserting on them cannot accidentally match a genuine release that leaked in
// from the network.
const (
	NodeVersion = "22.99.0"
	PnpmVersion = "9.99.0"
	YarnVersion = "1.99.0"
	BunVersion  = "1.2.99"

	// DriftrVersion is the "latest driftr" the fake release feed advertises.
	// It has to outrank any real release so self-update always has work to do.
	DriftrVersion = "99.0.0"
)

// nodePlatforms lists the os/arch pairs the node tarball is built for. It
// covers every platform driftr supports, so the same fixture works on a Linux
// runner and an Apple Silicon one.
var nodePlatforms = []struct{ os, arch string }{
	{"linux", "x64"},
	{"linux", "arm64"},
	{"darwin", "x64"},
	{"darwin", "arm64"},
}

// bunPlatforms lists bun's own os/arch naming, which differs from node's:
// bun calls arm64 "aarch64".
var bunPlatforms = []struct{ os, arch string }{
	{"linux", "x64"},
	{"linux", "aarch64"},
	{"darwin", "x64"},
	{"darwin", "aarch64"},
}

// goPlatforms lists the GOOS/GOARCH pairs driftr itself is released for. The
// updater builds its archive name from runtime.GOOS/runtime.GOARCH, so these
// use Go's names rather than node's or bun's.
var goPlatforms = []struct{ os, arch string }{
	{"linux", "amd64"},
	{"linux", "arm64"},
	{"darwin", "amd64"},
	{"darwin", "arm64"},
}

// tarFile is one entry in a generated tar archive.
type tarFile struct {
	name    string
	content string
}

// buildTarGz packs files into a gzipped tar. Entries are written in sorted
// order so the archive bytes — and therefore its checksum — are stable across
// runs.
func buildTarGz(files []tarFile) ([]byte, error) {
	sorted := append([]tarFile(nil), files...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].name < sorted[j].name })

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	for _, f := range sorted {
		hdr := &tar.Header{
			Name:     f.name,
			Mode:     0o755,
			Size:     int64(len(f.content)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, err
		}
		if _, err := tw.Write([]byte(f.content)); err != nil {
			return nil, err
		}
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// buildZip packs a single executable entry into a zip archive, the shape bun
// ships its releases in.
func buildZip(name, content string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	hdr := &zip.FileHeader{Name: name, Method: zip.Deflate}
	hdr.SetMode(0o755)
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write([]byte(content)); err != nil {
		return nil, err
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// nodeTarball builds a fake node release for one platform.
//
// bin/node prints the fixture version for -v/--version, and for any other
// first argument echoes that argument back. driftr runs pnpm and yarn by
// handing their script path to node, so echoing the argument is what lets a
// test confirm the shim exec'd pnpm.cjs rather than node's own -v.
func nodeTarball(osName, arch string) ([]byte, error) {
	prefix := fmt.Sprintf("node-v%s-%s-%s/", NodeVersion, osName, arch)
	return buildTarGz([]tarFile{
		{prefix + "bin/node", fmt.Sprintf(
			"#!/bin/sh\ncase \"$1\" in\n  -v|--version) echo \"v%s\" ;;\n  *) echo \"v%s ran: $1\" ;;\nesac\n",
			NodeVersion, NodeVersion)},
		{prefix + "bin/npm", "#!/bin/sh\necho \"10.99.0\"\n"},
		{prefix + "bin/npx", "#!/bin/sh\necho \"10.99.0\"\n"},
	})
}

// npmTarball builds a fake npm package tarball. npm tarballs carry a
// "package/" top-level directory, which driftr strips on extraction, so the
// script lands at <version dir>/bin/<script>.
func npmTarball(pkg, ver string, scripts []string) ([]byte, error) {
	files := make([]tarFile, 0, 1+len(scripts))
	files = append(files, tarFile{
		name:    "package/package.json",
		content: fmt.Sprintf("{\"name\":%q,\"version\":%q}\n", pkg, ver),
	})
	for _, script := range scripts {
		files = append(files, tarFile{
			name:    "package/bin/" + script,
			content: fmt.Sprintf("#!/usr/bin/env node\n// %s %s\n", pkg, ver),
		})
	}
	return buildTarGz(files)
}

// npmPackage is the registry metadata for one package. The tarball URL is
// filled in per request, because driftr refuses a tarball URL whose host does
// not match the registry it asked — and the fixture's host is only known once
// the test server is listening.
type npmPackage struct {
	name      string
	version   string
	tarball   []byte
	integrity string
}

// Handler returns the HTTP handler serving every fixture route.
func Handler() (http.Handler, error) {
	mux := http.NewServeMux()

	if err := registerNode(mux); err != nil {
		return nil, err
	}
	if err := registerNpm(mux); err != nil {
		return nil, err
	}
	if err := registerBun(mux); err != nil {
		return nil, err
	}
	if err := registerUpdate(mux); err != nil {
		return nil, err
	}
	return mux, nil
}

func registerNode(mux *http.ServeMux) error {
	tarballs := map[string][]byte{}
	var shasums bytes.Buffer

	for _, p := range nodePlatforms {
		data, err := nodeTarball(p.os, p.arch)
		if err != nil {
			return fmt.Errorf("build node tarball %s-%s: %w", p.os, p.arch, err)
		}
		filename := fmt.Sprintf("node-v%s-%s-%s.tar.gz", NodeVersion, p.os, p.arch)
		tarballs[filename] = data
		fmt.Fprintf(&shasums, "%x  %s\n", sha256.Sum256(data), filename)
	}

	// Marked LTS so `driftr install node@lts` resolves to something
	// end-to-end. LTS-vs-non-LTS filtering itself is covered by unit tests
	// against a mocked index in internal/installer/node_test.go.
	index := fmt.Sprintf(`[{"version":"v%s","lts":"Fixture"}]`, NodeVersion)

	mux.HandleFunc("GET /index.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, index)
	})

	versionPrefix := fmt.Sprintf("/v%s/", NodeVersion)
	mux.HandleFunc("GET "+versionPrefix+"SHASUMS256.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(shasums.Bytes())
	})
	mux.HandleFunc("GET "+versionPrefix+"{file}", func(w http.ResponseWriter, r *http.Request) {
		data, ok := tarballs[r.PathValue("file")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(data)
	})
	return nil
}

func registerNpm(mux *http.ServeMux) error {
	// The script names match what driftr expects to exec for each tool
	// (platform.toolBinaryMap). pnpm ships pnpx.cjs alongside pnpm.cjs, and
	// the pnpx shim resolves to it inside the same version directory.
	specs := []struct {
		pkg, ver string
		scripts  []string
	}{
		{"pnpm", PnpmVersion, []string{"pnpm.cjs", "pnpx.cjs"}},
		{"yarn", YarnVersion, []string{"yarn.js"}},
	}

	packages := map[string]*npmPackage{}
	for _, s := range specs {
		data, err := npmTarball(s.pkg, s.ver, s.scripts)
		if err != nil {
			return fmt.Errorf("build npm tarball %s: %w", s.pkg, err)
		}
		sum := sha512.Sum512(data)
		packages[s.pkg] = &npmPackage{
			name:      s.pkg,
			version:   s.ver,
			tarball:   data,
			integrity: "sha512-" + base64.StdEncoding.EncodeToString(sum[:]),
		}
	}

	// tarballURL points back at whatever host the request arrived on.
	// driftr validates that the tarball host matches the registry host, and
	// a test server's port is assigned at listen time.
	tarballURL := func(r *http.Request, p *npmPackage) string {
		return fmt.Sprintf("http://%s/registry/%s/-/%s-%s.tgz", r.Host, p.name, p.name, p.version)
	}

	lookup := func(w http.ResponseWriter, r *http.Request) (*npmPackage, bool) {
		p, ok := packages[r.PathValue("pkg")]
		if !ok {
			http.NotFound(w, r)
			return nil, false
		}
		return p, true
	}

	// Package metadata: dist-tags plus the full version map, which is what
	// driftr walks to resolve a partial spec like pnpm@9.
	mux.HandleFunc("GET /registry/{pkg}", func(w http.ResponseWriter, r *http.Request) {
		p, ok := lookup(w, r)
		if !ok {
			return
		}
		body := map[string]any{
			"name":      p.name,
			"dist-tags": map[string]string{"latest": p.version},
			"versions": map[string]any{
				p.version: versionMetadata(p, tarballURL(r, p)),
			},
		}
		writeJSON(w, body)
	})

	mux.HandleFunc("GET /registry/{pkg}/{version}", func(w http.ResponseWriter, r *http.Request) {
		p, ok := lookup(w, r)
		if !ok {
			return
		}
		if r.PathValue("version") != p.version {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, versionMetadata(p, tarballURL(r, p)))
	})

	mux.HandleFunc("GET /registry/{pkg}/-/{file}", func(w http.ResponseWriter, r *http.Request) {
		p, ok := lookup(w, r)
		if !ok {
			return
		}
		if r.PathValue("file") != fmt.Sprintf("%s-%s.tgz", p.name, p.version) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(p.tarball)
	})
	return nil
}

func versionMetadata(p *npmPackage, tarball string) map[string]any {
	return map[string]any{
		"name":    p.name,
		"version": p.version,
		"dist": map[string]string{
			"tarball":   tarball,
			"integrity": p.integrity,
		},
	}
}

func registerBun(mux *http.ServeMux) error {
	assets := map[string][]byte{}
	var shasums bytes.Buffer

	for _, p := range bunPlatforms {
		asset := fmt.Sprintf("bun-%s-%s.zip", p.os, p.arch)
		// bun's archive keeps the binary one directory down, named after the
		// asset: "bun-linux-x64/bun".
		entry := strings.TrimSuffix(asset, ".zip") + "/bun"
		data, err := buildZip(entry, fmt.Sprintf("#!/bin/sh\necho \"%s\"\n", BunVersion))
		if err != nil {
			return fmt.Errorf("build bun asset %s: %w", asset, err)
		}
		assets[asset] = data
		fmt.Fprintf(&shasums, "%x  %s\n", sha256.Sum256(data), asset)
	}

	releases := fmt.Sprintf(`[{"tag_name":"bun-v%s","draft":false,"prerelease":false}]`, BunVersion)

	mux.HandleFunc("GET /bun/releases", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, releases)
	})

	downloadPrefix := fmt.Sprintf("/bun/download/bun-v%s/", BunVersion)
	mux.HandleFunc("GET "+downloadPrefix+"SHASUMS256.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(shasums.Bytes())
	})
	mux.HandleFunc("GET "+downloadPrefix+"{file}", func(w http.ResponseWriter, r *http.Request) {
		data, ok := assets[r.PathValue("file")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(data)
	})
	return nil
}

// registerUpdate serves driftr's own release feed: the GitHub release index
// self-update reads the tag from, and the release archive plus checksums.txt
// it downloads. The archive holds a shell script named "driftr" — that is the
// name the updater extracts by, and running it proves the replacement landed.
func registerUpdate(mux *http.ServeMux) error {
	archives := map[string][]byte{}
	var checksums bytes.Buffer

	for _, p := range goPlatforms {
		name := fmt.Sprintf("driftr_%s_%s_%s.tar.gz", DriftrVersion, p.os, p.arch)
		data, err := buildTarGz([]tarFile{
			{"driftr", fmt.Sprintf("#!/bin/sh\necho \"driftr version %s\"\n", DriftrVersion)},
		})
		if err != nil {
			return fmt.Errorf("build driftr archive %s: %w", name, err)
		}
		archives[name] = data
		fmt.Fprintf(&checksums, "%x  %s\n", sha256.Sum256(data), name)
	}

	index := fmt.Sprintf(`{"tag_name":"v%s"}`, DriftrVersion)

	mux.HandleFunc("GET /update/api/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, index)
	})

	downloadPrefix := fmt.Sprintf("/update/download/v%s/", DriftrVersion)
	mux.HandleFunc("GET "+downloadPrefix+"checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(checksums.Bytes())
	})
	mux.HandleFunc("GET "+downloadPrefix+"{file}", func(w http.ResponseWriter, r *http.Request) {
		data, ok := archives[r.PathValue("file")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(data)
	})
	return nil
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}
