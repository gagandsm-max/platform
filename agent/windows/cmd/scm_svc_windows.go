//go:build windows

package main

import (
	"golang.org/x/sys/windows/svc"
)

// windowsSCMHandler adapts scmAdapter to golang.org/x/sys/windows/svc.Handler.
type windowsSCMHandler struct {
	adapter *scmAdapter
}

func (h *windowsSCMHandler) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	// Buffered channels avoid deadlock between status publication and control
	// delivery while the adapter is blocked on a pending status send.
	requests := make(chan scmRequest, 4)
	statuses := make(chan scmStatus, 16)

	var (
		svcSpecific bool
		errno       uint32
	)
	done := make(chan struct{})
	go func() {
		defer close(done)
		svcSpecific, errno = h.adapter.runSCMSession(requests, statuses)
	}()

	for {
		select {
		case <-done:
			for {
				select {
				case st := <-statuses:
					if shouldForwardSCMStatus(st) {
						changes <- toSvcStatus(st)
					}
				default:
					return svcSpecific, errno
				}
			}
		case st := <-statuses:
			if shouldForwardSCMStatus(st) {
				changes <- toSvcStatus(st)
			}
		case req := <-r:
			cr := scmRequest{CurrentStatus: fromSvcStatus(req.CurrentStatus)}
			switch req.Cmd {
			case svc.Interrogate:
				cr.Cmd = scmCmdInterrogate
			case svc.Stop:
				cr.Cmd = scmCmdStop
			case svc.Shutdown:
				cr.Cmd = scmCmdShutdown
			default:
				cr.Cmd = uint32(req.Cmd)
			}
			select {
			case requests <- cr:
			case <-done:
				// Session already finished; drop late controls.
			}
		}
	}
}

func toSvcStatus(st scmStatus) svc.Status {
	return svc.Status{
		State:                   svc.State(st.State),
		Accepts:                 svc.Accepted(st.Accepts),
		CheckPoint:              st.CheckPoint,
		WaitHint:                st.WaitHintMs,
		Win32ExitCode:           st.Win32ExitCode,
		ServiceSpecificExitCode: st.ServiceSpecificExitCode,
	}
}

func fromSvcStatus(st svc.Status) scmStatus {
	return scmStatus{
		State:                   uint32(st.State),
		Accepts:                 uint32(st.Accepts),
		CheckPoint:              st.CheckPoint,
		WaitHintMs:              st.WaitHint,
		Win32ExitCode:           st.Win32ExitCode,
		ServiceSpecificExitCode: st.ServiceSpecificExitCode,
	}
}
