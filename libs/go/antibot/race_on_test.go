//go:build race

package antibot_test

// raceOn skips latency assertions under -race (the detector slows code 5-10x).
const raceOn = true
