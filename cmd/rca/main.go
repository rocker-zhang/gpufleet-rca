// Command rca evaluates a normalized gpufleet.v1 EvidencePack (protojson) and
// prints the deterministic gpufleet.v1 Verdict (protojson). It reads the
// evidence window from a file argument or, with no argument (or "-"), from
// stdin — e.g. the open agent's /signals protojson output piped in.
//
// It performs NO LLM narration and NO cost attribution — those are server-side
// in the closed control plane (RULES §B). This bin only runs the open
// deterministic gate: FIRE a fault class with >=2 independent corroborating
// signals, else ABSTAIN.
package main

import (
	"fmt"
	"io"
	"os"

	gpufleetv1 "github.com/rocker-zhang/gpufleet-proto/gen/go/gpufleet/v1"
	"github.com/rocker-zhang/gpufleet-rca/registry"
	"google.golang.org/protobuf/encoding/protojson"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "rca: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	in, err := readInput(args, stdin)
	if err != nil {
		return err
	}

	var pack gpufleetv1.EvidencePack
	if err := protojson.Unmarshal(in, &pack); err != nil {
		return fmt.Errorf("bad EvidencePack protojson: %w", err)
	}

	verdict := registry.NewDefaultEngine().Evaluate(&pack)

	out, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(verdict)
	if err != nil {
		return fmt.Errorf("encode verdict: %w", err)
	}
	if _, err := stdout.Write(append(out, '\n')); err != nil {
		return err
	}
	return nil
}

// readInput reads the evidence window from a file argument, or from stdin when
// no argument is given or the argument is "-".
func readInput(args []string, stdin io.Reader) ([]byte, error) {
	if len(args) > 0 && args[0] != "-" {
		b, err := os.ReadFile(args[0])
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", args[0], err)
		}
		return b, nil
	}
	b, err := io.ReadAll(stdin)
	if err != nil {
		return nil, fmt.Errorf("read stdin: %w", err)
	}
	return b, nil
}
