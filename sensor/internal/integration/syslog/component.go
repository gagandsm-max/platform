package syslogintegration

import (
	"context"
	"fmt"

	protocolsyslog "platform/sensor/internal/protocol/syslog"
	sensorruntime "platform/sensor/internal/runtime"
)

type AssemblyConfig struct {
	UDP    protocolsyslog.UDPConfig
	Parser protocolsyslog.ParserConfig
	Sink   SinkConfig
}

type Component struct {
	receiver *protocolsyslog.UDPReceiver
	sink     *DurableSink
}

var _ sensorruntime.Component = (*Component)(nil)

func NewUDPComponent(cfg AssemblyConfig, durable SpoolAppender) (*Component, error) {
	parser, err := protocolsyslog.NewParser(cfg.Parser)
	if err != nil {
		return nil, err
	}
	sink, err := NewDurableSink(cfg.Sink, cfg.Parser, durable)
	if err != nil {
		return nil, err
	}
	receiver, err := protocolsyslog.NewUDPReceiver(cfg.UDP, parser, sink)
	if err != nil {
		return nil, err
	}
	return &Component{receiver: receiver, sink: sink}, nil
}

func RegisterUDP(rt *sensorruntime.Runtime, cfg AssemblyConfig, durable SpoolAppender) (*Component, error) {
	if rt == nil {
		return nil, fmt.Errorf("SYSLOG_INTEGRATION_INVALID_RUNTIME: runtime is required")
	}
	component, err := NewUDPComponent(cfg, durable)
	if err != nil {
		return nil, err
	}
	if err := rt.RegisterComponent(component); err != nil {
		return nil, err
	}
	return component, nil
}

func (c *Component) Name() string                         { return "syslog-udp" }
func (c *Component) Capability() sensorruntime.Capability { return sensorruntime.CapabilitySyslog }
func (c *Component) Start(ctx context.Context) error      { return c.receiver.Start(ctx) }
func (c *Component) Drain(ctx context.Context) error      { return c.receiver.Drain(ctx) }
func (c *Component) Shutdown(ctx context.Context) error   { return c.receiver.Shutdown(ctx) }
func (c *Component) Ready(context.Context) error {
	snap := c.receiver.Snapshot()
	if snap.State != protocolsyslog.ReceiverRunning {
		return &Error{Code: CodeInternal, Message: "Syslog UDP receiver is not ready"}
	}
	health := c.sink.Snapshot()
	if health.Degraded {
		code := health.LastErrorCode
		if code == "" {
			code = CodeStorageUnavailable
		}
		return &Error{Code: code, Message: "Syslog durable sink is degraded"}
	}
	return nil
}

type Snapshot struct {
	Receiver protocolsyslog.ReceiverSnapshot
	Sink     SinkSnapshot
}

func (c *Component) Snapshot() Snapshot {
	return Snapshot{Receiver: c.receiver.Snapshot(), Sink: c.sink.Snapshot()}
}
