package receiver

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"

	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/grpc"
	corev1 "k8s.io/api/core/v1"

	"github.com/rancher-sandbox/network-enforcer/internal/ownerkind"
	"github.com/rancher-sandbox/network-enforcer/internal/tlsutil"
	"github.com/rancher-sandbox/network-enforcer/internal/topology"
)

const targetMetricName = "obi.network.flow.bytes"

const (
	// the iface.direction OBI could return.
	directionIngress = "ingress"
	directionEgress  = "egress"
	directionUnknown = "unknown"

	// the direction we return when OBI returns unsupported direction.
	directionNotDefined = "not_defined"
)

type Receiver struct {
	colmetricspb.UnimplementedMetricsServiceServer

	store   *topology.Store
	port    int
	certDir string
	log     *slog.Logger
	server  *grpc.Server
}

// NewReceiver creates an OTLP metrics receiver. A non-empty certDir enables
// mTLS (RequireAndVerifyClientCert); empty runs the server insecure.
func NewReceiver(store *topology.Store, port int, certDir string, logger *slog.Logger) *Receiver {
	return &Receiver{
		store:   store,
		port:    port,
		certDir: certDir,
		log:     logger.With("component", "flowcollector"),
	}
}

func (r *Receiver) Start(ctx context.Context) error {
	listenerConfig := net.ListenConfig{}
	lis, err := listenerConfig.Listen(ctx, "tcp", fmt.Sprintf(":%d", r.port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", r.port, err)
	}

	var opts []grpc.ServerOption
	if r.certDir != "" {
		creds, credsErr := tlsutil.ServerCredentials(r.certDir)
		if credsErr != nil {
			return fmt.Errorf("failed to create OTLP receiver mTLS credentials: %w", credsErr)
		}
		opts = append(opts, grpc.Creds(creds))
		r.log.InfoContext(ctx, "OTLP receiver using mTLS", "cert_dir", r.certDir)
	} else {
		r.log.InfoContext(ctx, "OTLP receiver running in insecure mode (no mTLS)")
	}

	r.server = grpc.NewServer(opts...)
	colmetricspb.RegisterMetricsServiceServer(r.server, r)

	r.log.InfoContext(ctx, "listening", "port", r.port)

	go func() {
		<-ctx.Done()
		r.server.GracefulStop()
	}()

	if err = r.server.Serve(lis); err != nil {
		return fmt.Errorf("gRPC server error: %w", err)
	}
	return nil
}

func (r *Receiver) Export(
	ctx context.Context,
	req *colmetricspb.ExportMetricsServiceRequest,
) (*colmetricspb.ExportMetricsServiceResponse, error) {
	for _, rm := range req.GetResourceMetrics() {
		for _, sm := range rm.GetScopeMetrics() {
			for _, m := range sm.GetMetrics() {
				r.log.InfoContext(ctx, "received metric", "name", m.GetName())
				if m.GetName() != targetMetricName {
					continue
				}
				r.processMetric(m)
			}
		}
	}

	return &colmetricspb.ExportMetricsServiceResponse{}, nil
}

func (r *Receiver) processMetric(m *metricspb.Metric) {
	var dataPoints []*metricspb.NumberDataPoint

	switch d := m.GetData().(type) {
	case *metricspb.Metric_Sum:
		dataPoints = d.Sum.GetDataPoints()
	case *metricspb.Metric_Gauge:
		dataPoints = d.Gauge.GetDataPoints()
	default:
		return
	}

	for _, dp := range dataPoints {
		rec := r.generateFlow(attrMap(dp.GetAttributes()))
		if rec == nil {
			continue
		}
		r.store.Record(rec)
	}
}

func normalizeProtocol(protocol string) (corev1.Protocol, error) {
	p := corev1.Protocol(strings.ToUpper(protocol))

	switch p {
	case corev1.ProtocolTCP, corev1.ProtocolUDP:
		return p, nil
	case corev1.ProtocolSCTP:
		fallthrough
	default:
		return corev1.Protocol(""), fmt.Errorf("not supported protocol: %s", protocol)
	}
}

func (r *Receiver) generateFlow(attrs map[string]string) *topology.FlowRecord {
	// Example of a real TCP connection
	// client Pod -> server Service
	// {"attrs": {"client.port":"35796","direction":"request","dst.address":"10.96.18.232","dst.name":"http-service","dst.port":"80","iface.direction":"egress","k8s.dst.name":"http-service","k8s.dst.namespace":"default","k8s.dst.owner.name":"http-service","k8s.dst.owner.type":"Service","k8s.dst.type":"Service","k8s.src.name":"http-client-6d87bb58d7-v7jfc","k8s.src.namespace":"default","k8s.src.node.ip":"172.18.0.2","k8s.src.node.name":"kind-control-plane","k8s.src.owner.name":"http-client","k8s.src.owner.type":"Deployment","k8s.src.type":"Pod","network.protocol.name":"www","network.type":"ipv4","obi.ip":"172.18.0.2","server.port":"80","src.address":"10.0.0.245","src.name":"http-client-6d87bb58d7-v7jfc","src.port":"35796","transport":"TCP"}}
	//
	// client Pod -> server Pod
	// {"attrs": {"client.port":"35796","direction":"request","dst.address":"10.0.0.164","dst.name":"http-server-85d56547df-922sz","dst.port":"80","iface.direction":"egress","k8s.dst.name":"http-server-85d56547df-922sz","k8s.dst.namespace":"default","k8s.dst.node.ip":"172.18.0.2","k8s.dst.node.name":"kind-control-plane","k8s.dst.owner.name":"http-server","k8s.dst.owner.type":"Deployment","k8s.dst.type":"Pod","k8s.src.name":"http-client-6d87bb58d7-v7jfc","k8s.src.namespace":"default","k8s.src.node.ip":"172.18.0.2","k8s.src.node.name":"kind-control-plane","k8s.src.owner.name":"http-client","k8s.src.owner.type":"Deployment","k8s.src.type":"Pod","network.protocol.name":"www","network.type":"ipv4","obi.ip":"172.18.0.2","server.port":"80","src.address":"10.0.0.245","src.name":"http-client-6d87bb58d7-v7jfc","src.port":"35796","transport":"TCP"}}
	//
	// server Pod -> client Pod
	// {"attrs": {"client.port":"35796","direction":"response","dst.address":"10.0.0.245","dst.name":"http-client-6d87bb58d7-v7jfc","dst.port":"35796","iface.direction":"ingress","k8s.dst.name":"http-client-6d87bb58d7-v7jfc","k8s.dst.namespace":"default","k8s.dst.node.ip":"172.18.0.2","k8s.dst.node.name":"kind-control-plane","k8s.dst.owner.name":"http-client","k8s.dst.owner.type":"Deployment","k8s.dst.type":"Pod","k8s.src.name":"http-server-85d56547df-922sz","k8s.src.namespace":"default","k8s.src.node.ip":"172.18.0.2","k8s.src.node.name":"kind-control-plane","k8s.src.owner.name":"http-server","k8s.src.owner.type":"Deployment","k8s.src.type":"Pod","network.protocol.name":"www","network.type":"ipv4","obi.ip":"172.18.0.2","server.port":"80","src.address":"10.0.0.164","src.name":"http-server-85d56547df-922sz","src.port":"80","transport":"TCP"}}
	//
	// Server service -> client pod
	// {"attrs": {"client.port":"35796","direction":"response","dst.address":"10.0.0.245","dst.name":"http-client-6d87bb58d7-v7jfc","dst.port":"35796","iface.direction":"ingress","k8s.dst.name":"http-client-6d87bb58d7-v7jfc","k8s.dst.namespace":"default","k8s.dst.node.ip":"172.18.0.2","k8s.dst.node.name":"kind-control-plane","k8s.dst.owner.name":"http-client","k8s.dst.owner.type":"Deployment","k8s.dst.type":"Pod","k8s.src.name":"http-service","k8s.src.namespace":"default","k8s.src.owner.name":"http-service","k8s.src.owner.type":"Service","k8s.src.type":"Service","network.protocol.name":"www","network.type":"ipv4","obi.ip":"172.18.0.2","server.port":"80","src.address":"10.96.18.232","src.name":"http-service","src.port":"80","transport":"TCP"}}

	protocol, err := normalizeProtocol(attrs["transport"])
	if err != nil {
		r.log.Warn("skipping datapoint: missing protocol", "attrs", attrs)
		return nil
	}

	switch protocol {
	case corev1.ProtocolTCP:
		direction := normalizeDirection(attrs["iface.direction"])
		// If direction is ingress drop the flow for now, egress should be enough
		if direction != directionEgress {
			return nil
		}
	case corev1.ProtocolUDP:
		// Unfortunately, OBI has disabled the direction guessing on UDP flows by default in https://github.com/open-telemetry/opentelemetry-ebpf-instrumentation/pull/1519
		// Instead of relying on iface.direction, we implement a function to detect UDP server/client.
		// If we don't know for sure, both directions will be added in the proposals.
		var isIngress bool
		isIngress, err = isUDPIngressFlow(attrs)
		if err != nil {
			r.log.Warn("failed to check if we should skip udp flow", "error", err)
			return nil
		}
		if isIngress {
			r.log.Debug("skipping datapoint: udp ingress flow", "attrs", attrs)
			return nil
		}
	case corev1.ProtocolSCTP:
		fallthrough
	default:
		r.log.Warn("not supported protocol", "protocol", protocol)
		return nil
	}

	dstKindRaw := attrs["k8s.dst.owner.type"]
	// todo!: we need to generate the policy here because the service port could be different from the pod destination one. at the moment we ignore this case.
	if dstKindRaw == string(ownerkind.KindService) {
		return nil
	}

	// Even if we drop the service flow we can still have 2 flows that carries the same information.
	// srcPodIP -> dstPodIP (seen from src pod egress)
	// srcPodIP -> dstPodIP (seen from dst pod ingress)
	// we should end up with the same flow key in the table so it should be deduplicated by default.
	srcKindRaw := attrs["k8s.src.owner.type"]
	srcNs := attrs["k8s.src.namespace"]
	srcName := attrs["k8s.src.owner.name"]
	dstNs := attrs["k8s.dst.namespace"]
	dstName := attrs["k8s.dst.owner.name"]
	srcAddr := attrs["src.address"]
	dstAddr := attrs["dst.address"]
	dstPortStr := attrs["dst.port"]

	// todo!: It is not super clear why some flows don't have the workload information, for now we ignore them.
	// This is an example
	// 2026-06-11T14:49:30Z  INFO  flowcollector skipping datapoint: missing required fields {"attrs": {"client.port":"60360","direction":"request","dst.address":"10.0.0.2","dst.name":"opentelemetry-collector-59ccbf7448-5r4bv","dst.port":"13133","iface.direction":"egress","k8s.dst.name":"opentelemetry-collector-59ccbf7448-5r4bv","k8s.dst.namespace":"network-enforcer","k8s.dst.node.ip":"172.18.0.2","k8s.dst.node.name":"kind-control-plane","k8s.dst.owner.name":"opentelemetry-collector","k8s.dst.owner.type":"Deployment","k8s.dst.type":"Pod","network.protocol.name":"undefined","network.type":"ipv4","obi.ip":"172.18.0.2","server.port":"13133","src.address":"10.0.0.77","src.name":"10.0.0.77","src.port":"60360","transport":"TCP"}}
	if srcKindRaw == "" || srcNs == "" || srcName == "" ||
		dstKindRaw == "" || dstNs == "" || dstName == "" {
		r.log.Debug("skipping datapoint: missing required fields", "attrs", attrs)
		return nil
	}

	srcKind, srcSupported := ownerkind.IsValidEndpoint(srcKindRaw)
	dstKind, dstSupported := ownerkind.IsValidEndpoint(dstKindRaw)
	if !srcSupported || !dstSupported {
		r.log.Debug("skipping datapoint: unsupported workload kind",
			"srcKind", srcKindRaw,
			"dstKind", dstKindRaw)
		return nil
	}

	dstPort, err := strconv.ParseInt(dstPortStr, 10, 32)
	if err != nil || dstPort <= 0 || dstPort > 65535 {
		r.log.Warn("Dropped datapoint with missing or invalid dst.port", "value", dstPortStr)
		return nil
	}

	return &topology.FlowRecord{
		Source: topology.WorkloadKey{
			Namespace: srcNs,
			OwnerKind: srcKind,
			OwnerName: srcName,
		},
		Dest: topology.WorkloadKey{
			Namespace: dstNs,
			OwnerKind: dstKind,
			OwnerName: dstName,
		},
		DstPort:    int32(dstPort),
		Protocol:   protocol,
		SrcAddress: srcAddr,
		DstAddress: dstAddr,
	}
}

// isUDPIngressFlow returns true for UDP ingress flow (src port < 1024, dst port > 1024),
// which OBI cannot classify directionally — we skip those to avoid duplicate flows.
func isUDPIngressFlow(attrs map[string]string) (bool, error) {
	dstPortNum, err := strconv.Atoi(attrs["dst.port"])
	if err != nil {
		return false, fmt.Errorf("failed to convert dst.port to integer: %w", err)
	}
	srcPortNum, err := strconv.Atoi(attrs["src.port"])
	if err != nil {
		return false, fmt.Errorf("failed to convert src.port to integer: %w", err)
	}
	if srcPortNum < 1024 && dstPortNum > 1024 {
		return true, nil
	}
	return false, nil
}

func normalizeDirection(direction string) string {
	switch direction {
	case directionIngress:
		return directionIngress
	case directionEgress:
		return directionEgress
	case directionUnknown:
		return directionUnknown
	}
	return directionNotDefined
}

func attrMap(attrs []*commonpb.KeyValue) map[string]string {
	m := make(map[string]string, len(attrs))
	for _, kv := range attrs {
		v := kv.GetValue()
		if v == nil {
			continue
		}
		switch val := v.GetValue().(type) {
		case *commonpb.AnyValue_StringValue:
			if val.StringValue != "" {
				m[kv.GetKey()] = val.StringValue
			}
		case *commonpb.AnyValue_IntValue:
			m[kv.GetKey()] = strconv.FormatInt(val.IntValue, 10)
		case *commonpb.AnyValue_DoubleValue:
			m[kv.GetKey()] = strconv.FormatFloat(val.DoubleValue, 'f', -1, 64)
		case *commonpb.AnyValue_BoolValue:
			m[kv.GetKey()] = strconv.FormatBool(val.BoolValue)
		}
	}
	return m
}
