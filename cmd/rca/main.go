// Command rca evaluates a normalized signal window (JSON on stdin) against the
// deterministic public signatures and prints the Verdict. It performs NO LLM
// narration — that is server-side in the closed control plane.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/rocker-zhang/gpufleet-rca"
)

func main() {
	var w rca.Window
	if err := json.NewDecoder(os.Stdin).Decode(&w); err != nil {
		fmt.Fprintf(os.Stderr, "rca: bad window JSON on stdin: %v\n", err)
		os.Exit(2)
	}

	eng := rca.NewEngine(rca.XID79{})
	v := eng.Evaluate(w)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(struct {
		DeviceUUID   string   `json:"device_uuid"`
		Outcome      string   `json:"outcome"`
		FaultClass   string   `json:"fault_class,omitempty"`
		CitedSignals []string `json:"cited_signals,omitempty"`
	}{
		DeviceUUID:   v.DeviceUUID,
		Outcome:      v.Outcome.String(),
		FaultClass:   v.FaultClass,
		CitedSignals: v.CitedSignals,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "rca: encode failed: %v\n", err)
		os.Exit(1)
	}
}
