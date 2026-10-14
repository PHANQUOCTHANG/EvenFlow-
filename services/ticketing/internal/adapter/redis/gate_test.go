package redisadapter

import (
	"testing"
)

func TestGate_KeyFormatters(t *testing.T) {
	const ev = "event-123"
	const tt = "tt-456"

	if got := invKey(ev, tt); got != "inv:{event-123}:tt:tt-456" {
		t.Fatalf("invKey: got %q, expected %q", got, "inv:{event-123}:tt:tt-456")
	}

	if got := invPrefix(ev); got != "inv:{event-123}:tt:" {
		t.Fatalf("invPrefix: got %q, expected %q", got, "inv:{event-123}:tt:")
	}

	if got := holdZKey(ev); got != "hold:{event-123}:z" {
		t.Fatalf("holdZKey: got %q, expected %q", got, "hold:{event-123}:z")
	}

	if got := holdHKey(ev); got != "hold:{event-123}:h" {
		t.Fatalf("holdHKey: got %q, expected %q", got, "hold:{event-123}:h")
	}

	if got := userHoldKey(ev); got != "uhold:{event-123}" {
		t.Fatalf("userHoldKey: got %q, expected %q", got, "uhold:{event-123}")
	}

	if got := admittedKey(ev); got != "wr:{event-123}:admitted" {
		t.Fatalf("admittedKey: got %q, expected %q", got, "wr:{event-123}:admitted")
	}
}

func TestGate_ConversionHelpers(t *testing.T) {
	// asString
	if got := asString("hello"); got != "hello" {
		t.Fatalf("asString: got %q, expected %q", got, "hello")
	}
	if got := asString(123); got != "" {
		t.Fatalf("asString with int: got %q, expected empty string", got)
	}

	// asInt64
	if got := asInt64(int64(42)); got != 42 {
		t.Fatalf("asInt64 with int64: got %d, expected 42", got)
	}
	if got := asInt64("100"); got != 100 {
		t.Fatalf("asInt64 with string: got %d, expected 100", got)
	}
	if got := asInt64("invalid"); got != 0 {
		t.Fatalf("asInt64 with invalid string: got %d, expected 0", got)
	}
	if got := asInt64(true); got != 0 {
		t.Fatalf("asInt64 with bool: got %d, expected 0", got)
	}
}
