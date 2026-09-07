// Command fixture-server exposes the test fixture over a real port, for
// poking at driftr by hand against fake upstreams:
//
//	go run ./test/fixture-server &
//	DRIFTR_NODE_MIRROR=http://127.0.0.1:9123 \
//	DRIFTR_NPM_REGISTRY=http://127.0.0.1:9123/registry \
//	DRIFTR_BUN_RELEASES=http://127.0.0.1:9123/bun/releases \
//	DRIFTR_BUN_MIRROR=http://127.0.0.1:9123/bun/download \
//	driftr install node@22
//
// The e2e suite does not use this binary — it starts the same handler
// in-process (see e2e/e2e_test.go).
package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/stackmade/driftr/test/fixture"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:9123", "listen address")
	flag.Parse()

	handler, err := fixture.Handler()
	if err != nil {
		log.Fatalf("build fixture: %v", err)
	}

	log.Printf("fixture server listening on %s (node v%s, pnpm %s, yarn %s, bun %s)",
		*addr, fixture.NodeVersion, fixture.PnpmVersion, fixture.YarnVersion, fixture.BunVersion)
	log.Fatal(http.ListenAndServe(*addr, handler))
}
