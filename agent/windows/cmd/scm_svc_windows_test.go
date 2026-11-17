//go:build windows

package main

import (
	"errors"
	"testing"
	"time"

	"golang.org/x/sys/windows/svc"
)

// Execute is tested directly here. The x/sys svc.Run wrapper reports
// SERVICE_STOPPED after Execute returns; forwarding it from Execute first
// would close the SCM status handle twice.
func TestWindowsSCMHandlerDoesNotForwardTerminalStopped(t *testing.T) {
	for _, tc := range []struct {
		name         string
		life         *fakeLifecycle
		wantSpecific bool
		wantCode     uint32
		wantStates   []svc.State
	}{
		{
			name:       "clean_stop",
			life:       &fakeLifecycle{},
			wantStates: []svc.State{svc.StartPending, svc.Running, svc.StopPending},
		},
		{
			name:         "startup_failed",
			life:         &fakeLifecycle{startErr: errors.New("startup failed")},
			wantSpecific: true,
			wantCode:     exitCodeStartFailed,
			wantStates:   []svc.State{svc.StartPending},
		},
		{
			name:         "stop_failed",
			life:         &fakeLifecycle{stopErr: errors.New("shutdown failed")},
			wantSpecific: true,
			wantCode:     exitCodeStopFailed,
			wantStates:   []svc.State{svc.StartPending, svc.Running, svc.StopPending},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := &windowsSCMHandler{adapter: newSCMAdapter(tc.life, testLogger())}
			requests := make(chan svc.ChangeRequest, 1)
			changes := make(chan svc.Status, 16)
			type result struct {
				specific bool
				code     uint32
			}
			completed := make(chan result, 1)
			go func() {
				specific, code := handler.Execute(nil, requests, changes)
				completed <- result{specific: specific, code: code}
			}()

			var seen []svc.State
			deadline := time.After(3 * time.Second)
			stopSent := false
			for {
				select {
				case st := <-changes:
					if st.State == svc.Stopped {
						t.Fatal("Windows bridge must leave terminal STOPPED to svc.Run")
					}
					seen = append(seen, st.State)
					if st.State == svc.Running && !stopSent {
						stopSent = true
						requests <- svc.ChangeRequest{Cmd: svc.Stop}
					}
				case got := <-completed:
					for {
						select {
						case st := <-changes:
							if st.State == svc.Stopped {
								t.Fatal("terminal STOPPED forwarded after Execute completion")
							}
							seen = append(seen, st.State)
						default:
							if got.specific != tc.wantSpecific || got.code != tc.wantCode {
								t.Fatalf("exit: specific=%v code=%d want %v/%d", got.specific, got.code, tc.wantSpecific, tc.wantCode)
							}
							if len(seen) != len(tc.wantStates) {
								t.Fatalf("states=%v want=%v", seen, tc.wantStates)
							}
							for i := range seen {
								if seen[i] != tc.wantStates[i] {
									t.Fatalf("states=%v want=%v", seen, tc.wantStates)
								}
							}
							return
						}
					}
				case <-deadline:
					t.Fatalf("Windows SCM handler did not return: states=%v", seen)
				}
			}
		})
	}
}
