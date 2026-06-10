package rca

import (
	"reflect"
	"testing"
)

func TestXID79Gate(t *testing.T) {
	eng := NewEngine(XID79{})

	cases := []struct {
		name      string
		window    Window
		want      Outcome
		wantClass string
		wantCited []string
	}{
		{
			name: "two signals -> FIRE",
			window: Window{DeviceUUID: "GPU-1", Signals: []Signal{
				{Name: signalDmesgXid79, Detail: "Xid 79"},
				{Name: signalECCDBE, Detail: "dbe delta=3"},
			}},
			want:      Fire,
			wantClass: "xid79_gpu_fell_off_bus",
			wantCited: []string{signalDmesgXid79, signalECCDBE}, // sorted: "dmesg." < "ecc."
		},
		{
			name: "one signal (xid only) -> ABSTAIN",
			window: Window{DeviceUUID: "GPU-1", Signals: []Signal{
				{Name: signalDmesgXid79, Detail: "Xid 79"},
			}},
			want: Abstain,
		},
		{
			name: "one signal (ecc only) -> ABSTAIN",
			window: Window{DeviceUUID: "GPU-1", Signals: []Signal{
				{Name: signalECCDBE, Detail: "dbe delta=1"},
			}},
			want: Abstain,
		},
		{
			// Negative case proving FP=0: unrelated signals must NOT fire.
			name: "unrelated signals -> ABSTAIN (FP=0)",
			window: Window{DeviceUUID: "GPU-1", Signals: []Signal{
				{Name: "thermal.throttle", Detail: "tlimit"},
				{Name: "nccl.timeout", Detail: "ring"},
			}},
			want: Abstain,
		},
		{
			name:   "no signals -> ABSTAIN",
			window: Window{DeviceUUID: "GPU-1"},
			want:   Abstain,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := eng.Evaluate(tc.window)
			if v.Outcome != tc.want {
				t.Fatalf("outcome = %s, want %s", v.Outcome, tc.want)
			}
			if tc.want == Fire {
				if v.FaultClass != tc.wantClass {
					t.Errorf("class = %q, want %q", v.FaultClass, tc.wantClass)
				}
				if !reflect.DeepEqual(v.CitedSignals, tc.wantCited) {
					t.Errorf("cited = %v, want %v", v.CitedSignals, tc.wantCited)
				}
			} else if len(v.CitedSignals) != 0 || v.FaultClass != "" {
				t.Errorf("ABSTAIN must cite nothing and set no class, got %+v", v)
			}
		})
	}
}

func TestDeterminism(t *testing.T) {
	eng := NewEngine(XID79{})
	w := Window{DeviceUUID: "GPU-1", Signals: []Signal{
		{Name: signalECCDBE}, {Name: signalDmesgXid79},
	}}
	first := eng.Evaluate(w)
	for i := 0; i < 100; i++ {
		if got := eng.Evaluate(w); !reflect.DeepEqual(got, first) {
			t.Fatalf("non-deterministic verdict on iter %d: %+v != %+v", i, got, first)
		}
	}
}
