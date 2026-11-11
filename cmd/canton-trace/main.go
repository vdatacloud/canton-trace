// Command canton-trace prints a Canton transaction as a tree.
//
//	canton-trace -json-api http://localhost:7575 -party Alice::1220… <updateId>
//	canton-trace -grpc localhost:6865 -party Alice::1220… -o json <updateId>
//
// The bearer token, if the ledger needs one, is read from the environment
// variable -token-env names (CANTON_TOKEN by default), never from a flag, so
// it stays out of shell history and process listings.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	cantontrace "github.com/vdatacloud/canton-trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type partyList []string

func (p *partyList) String() string     { return strings.Join(*p, ",") }
func (p *partyList) Set(v string) error { *p = append(*p, v); return nil }

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "canton-trace:", err)
		if f := cantontrace.Explain(err); f != nil {
			out, _ := json.MarshalIndent(f, "", "  ")
			fmt.Fprintln(os.Stderr, string(out))
		}
		os.Exit(1)
	}
}

func run() error {
	var parties partyList
	jsonAPI := flag.String("json-api", "", "JSON Ledger API base URL, e.g. http://localhost:7575")
	grpcAddr := flag.String("grpc", "", "gRPC Ledger API address, e.g. localhost:6865")
	useTLS := flag.Bool("tls", false, "use TLS for -grpc")
	tokenEnv := flag.String("token-env", "CANTON_TOKEN", "environment variable holding the bearer token (unset: no token)")
	output := flag.String("o", "tree", "output: tree or json")
	timeout := flag.Duration("timeout", 30*time.Second, "request timeout")
	flag.Var(&parties, "party", "party to read as (repeatable)")
	flag.Parse()

	if flag.NArg() != 1 || len(parties) == 0 || (*jsonAPI == "") == (*grpcAddr == "") {
		flag.Usage()
		return errors.New("give one update id, at least one -party, and exactly one of -json-api or -grpc")
	}
	updateID := flag.Arg(0)
	token := os.Getenv(*tokenEnv)
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	var t *cantontrace.Trace
	var err error
	if *jsonAPI != "" {
		r := &cantontrace.JSONReader{BaseURL: *jsonAPI, Token: func(context.Context) (string, error) { return token, nil }}
		t, err = r.Read(ctx, updateID, parties)
	} else {
		creds := insecure.NewCredentials()
		if *useTLS {
			creds = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
		}
		conn, cerr := grpc.NewClient(*grpcAddr, grpc.WithTransportCredentials(creds))
		if cerr != nil {
			return fmt.Errorf("connecting to %s: %w", *grpcAddr, cerr)
		}
		defer func() { _ = conn.Close() }()
		if token != "" {
			ctx = cantontrace.WithBearer(ctx, token)
		}
		t, err = cantontrace.NewGRPCReader(conn).Read(ctx, updateID, parties)
	}
	if err != nil {
		return err
	}
	switch *output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(t)
	case "tree":
		return cantontrace.WriteText(os.Stdout, t)
	default:
		return fmt.Errorf("unknown output %q (tree or json)", *output)
	}
}
