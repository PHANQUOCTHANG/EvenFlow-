package antibot_test

import (
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/eventflow/eventflow/libs/go/antibot"
)

func benchInputs(t testing.TB) []antibot.Input {
	t.Helper()
	return []antibot.Input{
		input([]byte(validSignals)),
		input(nil),
		input(payload(t, map[string]any{"wd": true, "rt": 30, "pe": 1})),
		input(payload(t, map[string]any{"iv": repeatIv(32, 100, 100), "rt": 90})),
		input(payload(t, map[string]any{"gl": false, "fc": 1, "tz": "Europe/Berlin", "tzo": 120})),
	}
}

// BenchmarkEvaluate measures decode + pure engine (AC-5).
func BenchmarkEvaluate(b *testing.B) {
	cfg := cfgWith(b)
	ins := benchInputs(b)
	st := neutral()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		in := ins[i%len(ins)]
		sig, _ := antibot.DecodeSignals(in.RawSignals)
		_ = antibot.Evaluate(cfg, in, sig, st)
	}
}

// AC-5: the pure engine (decode + rules + score, no I/O) has p99 < 0.2 ms over 10 000 calls.
func TestEvaluate_AC5_P99(t *testing.T) {
	if raceOn || testing.Short() {
		t.Skip("latency assertion skipped under -race / -short")
	}
	cfg := cfgWith(t)
	ins := benchInputs(t)
	st := neutral()
	for i := 0; i < 1000; i++ { // warm caches (time zones, UA matcher)
		in := ins[i%len(ins)]
		sig, _ := antibot.DecodeSignals(in.RawSignals)
		antibot.Evaluate(cfg, in, sig, st)
	}
	// Windows' monotonic clock ticks in ~0.5 ms steps, so there each sample is the mean of a batch of 50 calls.
	batch := 1
	if runtime.GOOS == "windows" {
		batch = 50
	}
	const total = 10000
	n := total / batch
	d := make([]time.Duration, n)
	for i := range d {
		start := time.Now()
		for k := 0; k < batch; k++ {
			in := ins[(i*batch+k)%len(ins)]
			sig, _ := antibot.DecodeSignals(in.RawSignals)
			antibot.Evaluate(cfg, in, sig, st)
		}
		d[i] = time.Since(start) / time.Duration(batch)
	}
	slices.Sort(d)
	p99 := d[n*99/100]
	t.Logf("pure engine (batch %d) p50=%s p99=%s max=%s", batch, d[n/2], p99, d[n-1])
	if p99 >= 200*time.Microsecond {
		t.Fatalf("p99 = %s, want < 0.2 ms", p99)
	}
}
