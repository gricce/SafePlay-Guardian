package device

import "context"

type Registry interface {
	Get(ctx context.Context, id string) (*Device, error)
	List(ctx context.Context, f Filter) ([]Device, error)
	Upsert(ctx context.Context, d Device) (*Device, error)
	Reconcile(ctx context.Context, obs Observation) (deviceID string, err error)
}
