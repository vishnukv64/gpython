package py

import "testing"

func TestInterruptFlag(t *testing.T) {
	var f InterruptFlag
	if f.Requested() || f.Take() {
		t.Fatal("fresh flag must not be requested")
	}

	// An idle waiter is woken by the next Request.
	wake := f.Wake()
	select {
	case <-wake:
		t.Fatal("wake channel closed before Request")
	default:
	}
	f.Request()
	select {
	case <-wake:
	default:
		t.Fatal("Request did not wake the waiter")
	}
	if !f.Requested() {
		t.Fatal("Request not visible to Requested")
	}

	// Take delivers exactly once.
	if !f.Take() {
		t.Fatal("Take did not report the pending request")
	}
	if f.Take() || f.Requested() {
		t.Fatal("Take must clear the request")
	}

	// Wake while a request is pending returns an already-closed channel.
	f.Request()
	select {
	case <-f.Wake():
	default:
		t.Fatal("Wake with a pending request must be closed")
	}

	// Clear drops a pending request.
	f.Clear()
	if f.Requested() {
		t.Fatal("Clear did not drop the request")
	}
}
