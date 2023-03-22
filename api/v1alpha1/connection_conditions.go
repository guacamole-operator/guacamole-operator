package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConnectionConditionType is the type for a connection condition.
type ConnectionConditionType string

const (
	// ConnectionReady is the top-level health condition.
	ConnectionReady ConnectionConditionType = "Ready"
)

// ConnectionConditionReason is the reason type for a connection condition.
type ConnectionConditionReason string

const (
	// ConnectionReconciling is the reason when a connection is reconciling.
	ConnectionReconciling ConnectionConditionReason = "Reconciling"
	// ConnectionSynced is the reason when a connection is synced.
	ConnectionSynced ConnectionConditionReason = "Synchronized"
	// ConnectionSynced is the reason when a connection is out of sync.
	// This can be the result of a failed create / update operation or a
	// problem with the Guacamole API connection and therefore missing
	// information about the synchronization status.
	ConnectionUnsynced ConnectionConditionReason = "Unsynchronized"
)

// MarkAsUnknown sets the ready condition to unknown.
// Indicates that a connection is not yet processed.
func (s *ConnectionStatus) MarkAsUnknown() {
	meta.SetStatusCondition(&s.Conditions, metav1.Condition{
		Type:    string(ConnectionReady),
		Reason:  string(ConnectionReconciling),
		Status:  metav1.ConditionUnknown,
		Message: "Starting reconciliation.",
	})
}

// MarkAsSynchronized sets the ready condition to true.
// Indicates that a connection is synchronized.
func (s *ConnectionStatus) MarkAsSynchronized() {
	meta.SetStatusCondition(&s.Conditions, metav1.Condition{
		Type:    string(ConnectionReady),
		Reason:  string(ConnectionSynced),
		Status:  metav1.ConditionTrue,
		Message: "Connection synchronized.",
	})
}

// MarkAsUnsynchronized sets the ready condition to false.
// Indicates that a connection is not synchronized.
func (s *ConnectionStatus) MarkAsUnsynchronized() {
	meta.SetStatusCondition(&s.Conditions, metav1.Condition{
		Type:    string(ConnectionReady),
		Reason:  string(ConnectionUnsynced),
		Status:  metav1.ConditionFalse,
		Message: "Connection unsynchronized.",
	})
}
