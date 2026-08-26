package payment

import "testing"

func TestCaptureStateMachine(t *testing.T) {
	tests := []struct {
		status         Status
		auth, got, add int64
		want           Status
		err            error
	}{{Authorized, 100, 0, 100, Captured, nil}, {Authorized, 100, 0, 40, PartiallyCaptured, nil}, {PartiallyCaptured, 100, 40, 60, Captured, nil}, {Created, 100, 0, 1, "", ErrInvalidTransition}, {Authorized, 100, 90, 11, "", ErrOverCapture}}
	for _, tt := range tests {
		got, err := CaptureStatus(tt.status, tt.auth, tt.got, tt.add)
		if got != tt.want || (err != nil) != (tt.err != nil) {
			t.Fatalf("%+v: got %s %v", tt, got, err)
		}
	}
}
func TestAuthorizeOnlyCreated(t *testing.T) {
	if CanAuthorize(Authorized) == nil {
		t.Fatal("allowed invalid transition")
	}
}
