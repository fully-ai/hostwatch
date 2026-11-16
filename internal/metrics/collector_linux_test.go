//go:build linux

package metrics

import "testing"

func TestLinuxProcParsersAndLiveSample(t *testing.T) {
	cpu, err := parseProcStat("cpu  100 0 50 850 0 0 0 0 0 0\n")
	if err != nil || cpu.total != 1000 || cpu.idle != 850 {
		t.Fatalf("unexpected proc stat parse: %+v err=%v", cpu, err)
	}

	tot, used := parseMeminfo("MemTotal:       1000 kB\nMemAvailable:    250 kB\n")
	if tot != 1000*1024 || used != 750*1024 {
		t.Fatalf("unexpected meminfo parse: tot=%d used=%d", tot, used)
	}

	rx, tx := parseNetDev("  lo: 999 0 0 0 0 0 0 0 999 0\neth0: 1200 0 0 0 0 0 0 0 800 0\n")
	if rx != 1200 || tx != 800 {
		t.Fatalf("unexpected net dev parse: rx=%d tx=%d", rx, tx)
	}

	sample, err := CollectSample("test-host", []string{"nonexistent-svc-xyz"}, "/")
	if err != nil {
		t.Fatal(err)
	}
	if sample.HostID != "test-host" || sample.MemTotalBytes == 0 || sample.DiskTotalBytes == 0 {
		t.Fatalf("unexpected live sample: %+v", sample)
	}
}
