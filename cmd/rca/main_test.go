package main

import (
	"bytes"
	"strings"
	"testing"
)

const twoSignalPack = `{
  "contractVersion": "v1",
  "timeline": [
    {"source": "SIGNAL_SOURCE_DMESG_XID", "signalId": "dmesg.xid79", "label": "Xid 79"},
    {"source": "SIGNAL_SOURCE_DCGM", "signalId": "device.lost.dcgm", "label": "device unreachable"}
  ]
}`

const oneSignalPack = `{
  "contractVersion": "v1",
  "timeline": [
    {"source": "SIGNAL_SOURCE_DMESG_XID", "signalId": "dmesg.xid79", "label": "Xid 79"}
  ]
}`

func TestRun_TwoSignalsFire(t *testing.T) {
	var out bytes.Buffer
	if err := run(nil, strings.NewReader(twoSignalPack), &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	s := out.String()
	for _, want := range []string{
		"FAULT_CLASS_GPU_FALLEN_OFF_BUS",
		"GATE_SIGNATURE_XID79_FALLEN_OFF_BUS",
		"dmesg.xid79",
		"device.lost.dcgm",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("verdict output missing %q:\n%s", want, s)
		}
	}
}

func TestRun_OneSignalAbstains(t *testing.T) {
	var out bytes.Buffer
	if err := run(nil, strings.NewReader(oneSignalPack), &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "FAULT_CLASS_ABSTAIN") {
		t.Errorf("want ABSTAIN, got:\n%s", s)
	}
	if strings.Contains(s, "citedSignals") {
		t.Errorf("ABSTAIN must not cite signals:\n%s", s)
	}
}
