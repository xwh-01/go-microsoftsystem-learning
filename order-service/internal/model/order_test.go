package model

import "testing"

func TestStatusFromStockResult(t *testing.T) {
	if got := StatusFromStockResult(true); got != OrderStatusConfirmed {
		t.Fatalf("success result should become %s, got %s", OrderStatusConfirmed, got)
	}

	if got := StatusFromStockResult(false); got != OrderStatusFailed {
		t.Fatalf("failed result should become %s, got %s", OrderStatusFailed, got)
	}
}

func TestMessageFromStockResult(t *testing.T) {
	if got := MessageFromStockResult(true, "ignored"); got != OrderMessageConfirmed {
		t.Fatalf("success message should be %q, got %q", OrderMessageConfirmed, got)
	}

	if got := MessageFromStockResult(false, "insufficient stock"); got != "insufficient stock" {
		t.Fatalf("failed result should keep reason, got %q", got)
	}

	if got := MessageFromStockResult(false, ""); got != OrderMessageFailed {
		t.Fatalf("failed result without reason should use default message, got %q", got)
	}
}
