# Graph Report - canton-trace  (2026-11-11)

## Corpus Check
- 25 files · ~33,623 words
- Verdict: corpus is large enough that graph structure adds value.
- Unclassified: 4 file(s) not represented in the graph (top: (none) 4)

## Summary
- 249 nodes · 537 edges · 17 communities (12 shown, 5 thin omitted)
- Extraction: 93% EXTRACTED · 7% INFERRED · 0% AMBIGUOUS · INFERRED: 40 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `0307ede0`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- grpc_test.go
- handler_test.go
- Trace
- value.go
- build_test.go
- GitHub & CI Workflow Skill
- Handler
- grpc.go
- canton-trace
- Repository Guardrails
- CLAUDE.md
- WriteText
- test-next-version.sh
- graphify
- install-git-hooks.sh
- next-version.sh
- github.com/vdatacloud/canton-trace

## God Nodes (most connected - your core abstractions)
1. `Handler` - 31 edges
2. `Trace` - 17 edges
3. `Explain()` - 13 edges
4. `GitHub & CI Workflow Skill` - 12 edges
5. `Build()` - 9 edges
6. `shape()` - 9 edges
7. `FromTransaction()` - 9 edges
8. `fakeLedger` - 9 edges
9. `canton-trace` - 9 edges
10. `FromPrepared()` - 8 edges

## Surprising Connections (you probably didn't know these)
- `run()` --calls--> `WriteText()`  [EXTRACTED]
  cmd/canton-trace/main.go → render.go
- `ErrorBody` --references--> `Failure`  [EXTRACTED]
  traceapi/handler.go → explain.go
- `writeLedgerError()` --calls--> `Explain()`  [EXTRACTED]
  traceapi/handler.go → explain.go
- `Handler` --references--> `Reader`  [EXTRACTED]
  traceapi/handler.go → render.go
- `Handler` --references--> `Trace`  [EXTRACTED]
  traceapi/handler.go → trace.go

## Import Cycles
- None detected.

## Communities (17 total, 5 thin omitted)

### Community 0 - "grpc_test.go"
Cohesion: 0.09
Nodes (29): TestShortID(), JSONError, main(), run(), codeFromMessage(), Explain(), explainJSON(), unwrapToStatus() (+21 more)

### Community 2 - "Trace"
Cohesion: 0.10
Nodes (9): fakeLedger, GRPCReader, JSONReader, Kind, Reader, Source, Trace, Node (+1 more)

### Community 3 - "value.go"
Cohesion: 0.13
Nodes (18): jsonEvent, kv, orderedObject, nodeFromEvent(), optionalValue(), sampleTx(), IdentifierString(), recordAny() (+10 more)

### Community 4 - "build_test.go"
Cohesion: 0.16
Nodes (17): Build(), cr(), ex(), shape(), TestBuild_LeafExerciseAndNils(), TestBuild_NestedExercises(), TestBuild_OrderOfInputDoesNotMatter(), TestBuild_ProjectionWithoutTheParent() (+9 more)

### Community 5 - "GitHub & CI Workflow Skill"
Cohesion: 0.11
Nodes (17): 10. Useful Reference Commands, 11. References, 1. Pre-Commit Local Verification (MANDATORY — do this before every commit), 2. Branching Rules, 3. Commit Standards, 4. Staging & Pushing Changes, 5. Pull Request Creation, 6. CI Pipeline Overview (+9 more)

### Community 6 - "Handler"
Cohesion: 0.27
Nodes (11): Failure, BadRequestError, ErrorBody, Handler, grpcCode(), noStore(), viewerFromAuth(), writeError() (+3 more)

### Community 7 - "grpc.go"
Cohesion: 0.16
Nodes (4): Simulator, FromPrepared(), minus(), NewSimulator()

### Community 8 - "canton-trace"
Cohesion: 0.14
Nodes (12): canton-trace, CLI, Credits, Development, HTTP, Install, Library, License (+4 more)

### Community 9 - "Repository Guardrails"
Cohesion: 0.22
Nodes (8): Branch Protection Strategy, Branching Strategy, CI Requirements, Code Review Requirements, Commit Standard, Pre-Commit Verification, Pull Request Rules, Repository Guardrails

### Community 10 - "CLAUDE.md"
Cohesion: 0.29
Nodes (5): Commands, Git, Layout, Project, Rules

### Community 11 - "WriteText"
Cohesion: 0.40
Nodes (3): entity(), Node, WriteText()

### Community 12 - "test-next-version.sh"
Cohesion: 0.70
Nodes (4): c(), expect(), test-next-version.sh script, tg()

## Knowledge Gaps
- **42 isolated node(s):** `github.com/vdatacloud/canton-trace`, `Node`, `install-git-hooks.sh script`, `next-version.sh script`, `kv` (+37 more)
  These have ≤1 connection - possible missing edges or undocumented components. (Counts symbols only; 79 node(s) total have ≤1 connection when file, concept and rationale nodes are included.)
- **5 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Handler` connect `Handler` to `grpc_test.go`, `handler_test.go`, `Trace`, `value.go`, `build_test.go`, `grpc.go`?**
  _High betweenness centrality (0.129) - this node is a cross-community bridge._
- **Are the 6 inferred relationships involving `Explain()` (e.g. with `.Error()` and `TestExplain_NotALedgerError()`) actually correct?**
  _`Explain()` has 6 INFERRED edges - model-reasoned connections that need verification._
- **What connects `github.com/vdatacloud/canton-trace`, `Node`, `install-git-hooks.sh script` to the rest of the system?**
  _42 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `grpc_test.go` be split into smaller, more focused modules?**
  _Cohesion score 0.0931174089068826 - nodes in this community are weakly interconnected._
- **Why does `Trace` connect `Trace` to `WriteText`, `build_test.go`, `Handler`, `grpc.go`?**
  _High betweenness centrality (0.054) - this node is a cross-community bridge._
- **Should `handler_test.go` be split into smaller, more focused modules?**
  _Cohesion score 0.13793103448275862 - nodes in this community are weakly interconnected._
- **Why does `fakeLedger` connect `Trace` to `grpc_test.go`?**
  _High betweenness centrality (0.037) - this node is a cross-community bridge._