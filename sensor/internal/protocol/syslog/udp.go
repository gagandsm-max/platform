package syslog

import (
	"errors"
	"net"
	"time"
)

func (r *UDPReceiver) readLoop() {
	defer close(r.readerDone)
	defer r.closeQueue()
	defer r.closeConn()

	buf := make([]byte, r.cfg.MaxDatagramBytes+1)
	for {
		ctx := r.context()
		if err := ctx.Err(); err != nil {
			r.setLastError(contextCode(err))
			return
		}
		if err := r.conn.SetReadDeadline(time.Now().Add(r.cfg.ReadTimeout)); err != nil {
			if r.isStopping() || errors.Is(err, net.ErrClosed) {
				return
			}
			r.setFailure(CodeInternal)
			return
		}

		n, source, err := r.conn.ReadFromUDP(buf)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				r.setLastError(contextCode(ctxErr))
				return
			}
			if r.isStopping() || errors.Is(err, net.ErrClosed) {
				return
			}
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				continue
			}
			r.setFailure(CodeInternal)
			return
		}

		r.metrics.datagramsReceived.Add(1)
		if n > r.cfg.MaxDatagramBytes {
			r.metrics.oversized.Add(1)
			r.setLastError(CodeMessageTooLarge)
			continue
		}

		payload := make([]byte, n)
		copy(payload, buf[:n])
		r.enqueue(datagram{
			payload:    payload,
			receivedAt: time.Now().UTC(),
			source:     source.String(),
		})
	}
}

func (r *UDPReceiver) enqueue(d datagram) bool {
	select {
	case r.queue <- d:
		return true
	default:
		r.metrics.queueFullDrops.Add(1)
		r.setLastError(CodeQueueFull)
		return false
	}
}
