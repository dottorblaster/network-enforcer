package grpcexporter

import (
	"context"
	"time"

	agentv1 "github.com/rancher-sandbox/network-enforcer/proto/agent/v1"
	"google.golang.org/grpc"
)

const (
	agentClientTimeout = 30 * time.Second
)

// AgentClientAPI defines the interface for interacting with a cniwatcher gRPC
// agent. Implementations can be real gRPC clients or test fakes.
type AgentClientAPI interface {
	// ScrapeViolations drains the cniwatcher's violation buffer and returns all
	// accumulated records since the last scrape.
	ScrapeViolations(ctx context.Context) ([]*agentv1.ViolationRecord, error)
	// Close tears down the underlying connection.
	Close() error
}

// AgentClient is the production implementation of AgentClientAPI backed by a
// gRPC connection to a cniwatcher pod.
type AgentClient struct {
	conn    *grpc.ClientConn
	client  agentv1.NetworkAgentClient
	timeout time.Duration
}

// NewAgentClient creates a new AgentClient from an existing gRPC connection.
func NewAgentClient(conn *grpc.ClientConn) *AgentClient {
	return &AgentClient{
		conn:    conn,
		client:  agentv1.NewNetworkAgentClient(conn),
		timeout: agentClientTimeout,
	}
}

// ScrapeViolations drains the cniwatcher's violation buffer and returns all
// accumulated records.
func (c *AgentClient) ScrapeViolations(ctx context.Context) ([]*agentv1.ViolationRecord, error) {
	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, c.timeout)
	defer timeoutCancel()

	resp, err := c.client.ScrapeViolations(timeoutCtx, &agentv1.ScrapeViolationsRequest{})
	if err != nil {
		return nil, err
	}
	return resp.GetViolations(), nil
}

// Close closes the underlying gRPC connection.
func (c *AgentClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
