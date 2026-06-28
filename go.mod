module github.com/rocker-zhang/gpufleet-rca

go 1.26.0

require (
	github.com/rocker-zhang/gpufleet-proto/gen/go v0.1.0
	google.golang.org/protobuf v1.36.11
)

require (
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260226221140-a57be14db171 // indirect
	google.golang.org/grpc v1.81.1 // indirect
)

// Poly-repo: in CI this dependency is consumed at the pinned proto tag
// (proto/v0.1.0). For local workspace builds this replace points at the sibling
// repo so the build resolves offline against the vendored REAL gen types — NOT
// a hand-rolled mirror. Matches the agent / cli / semantics modules' convention.
