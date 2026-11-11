# canton-trace

Read a Canton / Daml ledger transaction as a **tree**: which exercise caused
which create, archive or nested exercise, with arguments and results, as the
reading party is allowed to see it.

The Ledger API v2 returns a transaction as a flat list of events. Each
exercise carries `last_descendant_node_id`, and node ids are pre-order, so
one pass with a stack rebuilds the tree. This module does that, reads
transactions over either transport, explains ledger errors, and serves traces
over HTTP.

- **Library** (`github.com/vdatacloud/canton-trace`):
  - `Build` rebuilds the tree from flat events.
  - `FromTransaction` adapts gRPC, `ParseJSONTransaction` adapts the JSON
    Ledger API, and `FromPrepared` adapts an interactive-submission prepared
    transaction.
  - `GRPCReader` and `JSONReader` read an update by id.
  - `Simulator` runs `PrepareSubmission`.
  - `Explain` reads a ledger error into its code, category and correlation id.
- **HTTP** (`traceapi`): `GET …/{updateId}` and `POST …/simulate` handlers.
- **CLI** (`cmd/canton-trace`): prints a trace as a tree or JSON.

Both transports produce identical traces. Values use the JSON Ledger API's
encoding on both, so a trace read over gRPC equals the same trace read over
JSON.

## Install

```sh
go get github.com/vdatacloud/canton-trace
go install github.com/vdatacloud/canton-trace/cmd/canton-trace@latest
```

Requires Go 1.27 and a Canton 3.x participant (Ledger API v2). gRPC stubs
come from [dazl-client](https://github.com/digital-asset/dazl-client) v8.

## CLI

```sh
export CANTON_TOKEN=…   # omit for an unsecured ledger
canton-trace -json-api http://localhost:7575 -party 'alice::1220…' 1220abcd…
canton-trace -grpc localhost:6865 -party 'alice::1220…' -o json 1220abcd…
```

```
1220abcd…  6 nodes
  #0 exercise Demo.Escrow:Escrow.Release (consuming) 00e1f2a3b4…
    #1 exercise Demo.Token:Factory.Transfer via Api.Token:TransferFactory 00f9e8d7c6…
      #2 create Demo.Token:Holding 00a1b2c3d4…
      #3 exercise Demo.Token:Holding.Archive (consuming) 0077665544…
    #4 create Demo.Escrow:Receipt 00c0ffee00…
```

The token is read from an environment variable (`-token-env`, default
`CANTON_TOKEN`), never from a flag. Ledger errors print with their Canton
code, category and correlation id.

## Library

```go
conn, _ := grpc.NewClient("localhost:6865", grpc.WithTransportCredentials(insecure.NewCredentials()))
r := cantontrace.NewGRPCReader(conn)
t, err := r.Read(cantontrace.WithBearer(ctx, token), updateID, []string{party})
if err != nil {
	if f := cantontrace.Explain(err); f != nil {
		log.Printf("ledger refused: %s (correlation %s)", f.Code, f.CorrelationID)
	}
	return err
}
t.Walk(func(n *cantontrace.Node, depth int) bool {
	fmt.Println(strings.Repeat("  ", depth) + n.Line())
	return true
})
```

### What a reader sees

A read returns only the events the requesting parties may see. A visible
node whose parent is hidden attaches to its nearest visible ancestor, or
becomes a root. The tree is never deeper than the ledger's, and no visible
event is lost. This module never widens a read: the parties come from the
caller, and the ledger enforces the projection.

### Simulation

`Simulator.Simulate` calls `InteractiveSubmissionService.PrepareSubmission`
and returns the transaction a command would commit, or the ledger's error
(for example `DAML_AUTHORIZATION_ERROR`). Nothing is signed or submitted.
Prepared node ids are local to the prepared transaction.

## HTTP

```go
mux.Handle("GET /api/v1/traces/{updateId}", &traceapi.Handler{
	Reader:   reader,
	Viewer:   func(r *http.Request) ([]string, error) { return partiesOfVerifiedUser(r) },
	Decorate: func(r *http.Request, t *cantontrace.Trace) error { return redactForViewer(r, t) },
})
mux.Handle("POST /api/v1/traces/simulate", &traceapi.SimulateHandler{Prepare: prepareFromBody})
```

Design rules the handlers enforce:

- **The caller never picks the party.** `Viewer` resolves parties from the
  application's own verified authentication. A header such as
  `X-Canton-Party` would let any caller read as anyone the server's
  credentials can act for, so the handler ignores such headers.
- **No caching.** Every response is `Cache-Control: no-store`.
- **No leaks in errors.** Ledger error text can name parties and contracts,
  so it is left out unless `LedgerMessages` is set. The Canton code and
  correlation id are always kept.
- **Display is the application's job.** `Decorate` runs before the trace is
  written; use it to swap party ids for names or drop fields a viewer
  shouldn't read. JSON carries full ids. `ShortID` is for display only.

## Development

```sh
make verify          # build, vet, unit tests (no network, no ledger)
make lint            # golangci-lint
make install-hooks   # pre-push runs make verify
```

Releases are automatic on merge to `main` (see [RELEASING.md](RELEASING.md)).
Tests use synthetic fixtures only.

## Credits

The tree reconstruction follows the approach of
[walnuthq/dpm-trace](https://github.com/walnuthq/dpm-trace): rebuild the
call tree from `last_descendant_node_id`. This is an independent Go
implementation.

## License

Apache-2.0. See [LICENSE](LICENSE).
