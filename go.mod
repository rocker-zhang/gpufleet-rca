module github.com/rocker-zhang/gpufleet-rca

go 1.26.0

require (
	github.com/rocker-zhang/gpufleet-proto/gen/go v0.3.0
	google.golang.org/protobuf v1.36.11
)

require (
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260226221140-a57be14db171 // indirect
	google.golang.org/grpc v1.81.1 // indirect
)

// Poly-repo: gpufleet-proto/gen/go is consumed as a published module (no local
// replace). The pinned tag gen/go/v0.3.0 is the release path; the ci/verify-by-tag.sh
// script confirms the tag resolves from the Go proxy and guards against a stray
// replace directive creeping in. Matches the agent / cli / semantics convention.
