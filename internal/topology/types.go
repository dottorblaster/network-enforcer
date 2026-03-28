package topology

import "time"

// SupportedKinds contains the workload kinds that the operator manages.
var SupportedWorkloadTypes = map[string]bool{
	"Deployment":  true,
	"StatefulSet": true,
	"DaemonSet":   true,
}

type WorkloadKey struct {
	Namespace string
	OwnerKind string
	OwnerName string
}

type FlowRecord struct {
	Source   WorkloadKey
	Dest     WorkloadKey
	DstPort  int32
	Protocol string // TCP or UDP

	SrcAddress string
	DstAddress string

	FirstSeen time.Time
	LastSeen  time.Time
	ByteCount int64
}

type flowKey struct {
	Source   WorkloadKey
	Dest     WorkloadKey
	DstPort  int32
	Protocol string
}
