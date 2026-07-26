// Command openapi-public generates the published OpenAPI document from the
// hand-maintained full one, by dropping every operation marked
// `x-audience: internal` and pruning what nothing references any more.
//
//	go run ./cmd/openapi-public              # rewrite api/openapi.public.yaml
//	go run ./cmd/openapi-public -check       # fail if it is out of date
//
// The checked-in output is verified byte-for-byte by
// TestPublicSpecIsUpToDate in internal/handler, so CI catches drift whether or
// not this command is run.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"

	"github.com/williamokano/go-torrent-trader/backend/internal/openapi"
)

func run() int {
	in := flag.String("in", "api/openapi.yaml", "path to the full OpenAPI spec")
	out := flag.String("out", "api/openapi.public.yaml", "path to write the public OpenAPI spec to")
	check := flag.Bool("check", false, "verify the output is up to date instead of writing it")
	flag.Parse()

	src, err := os.ReadFile(*in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "openapi-public: reading %s: %v\n", *in, err)
		return 1
	}

	generated, err := openapi.Public(src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "openapi-public: generating from %s: %v\n", *in, err)
		return 1
	}

	if *check {
		current, err := os.ReadFile(*out)
		if err != nil {
			fmt.Fprintf(os.Stderr, "openapi-public: reading %s: %v\n", *out, err)
			return 1
		}
		if !bytes.Equal(current, generated) {
			fmt.Fprintf(os.Stderr, "openapi-public: %s is out of date — run: task generate:openapi\n", *out)
			return 1
		}
		fmt.Printf("openapi-public: %s is up to date\n", *out)
		return 0
	}

	if err := os.WriteFile(*out, generated, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "openapi-public: writing %s: %v\n", *out, err)
		return 1
	}
	fmt.Printf("openapi-public: wrote %s from %s\n", *out, *in)
	return 0
}

func main() {
	os.Exit(run())
}
