package network

import (
	"context"
	"fmt"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stuttgart-things/clusterbook/provider/apis/v1alpha1"
	"github.com/stuttgart-things/clusterbook/provider/internal/client"
)

const (
	errNotNetwork = "managed resource is not a Network"
	errGetPC      = "cannot get ProviderConfig"
	errObserve    = "cannot observe Network"
	errCreate     = "cannot create Network"
	errUpdate     = "cannot update Network"
	errDelete     = "cannot delete Network"
)

// Setup adds the Network controller to the manager.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.NetworkGroupKind)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		For(&v1alpha1.Network{}).
		Complete(managed.NewReconciler(mgr,
			resource.ManagedKind(v1alpha1.NetworkGroupVersionKind),
			managed.WithExternalConnecter(&connector{kube: mgr.GetClient()}),
			managed.WithLogger(o.Logger.WithValues("controller", name)),
			managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		))
}

type connector struct {
	kube k8sclient.Client
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.Network)
	if !ok {
		return nil, errors.New(errNotNetwork)
	}

	ref := cr.GetProviderConfigReference()
	if ref == nil {
		return nil, errors.New("no providerConfigRef set")
	}

	pc := &v1alpha1.ProviderConfig{}
	if err := c.kube.Get(ctx, k8sclient.ObjectKey{Name: ref.Name}, pc); err != nil {
		return nil, errors.Wrap(err, errGetPC)
	}

	return &external{client: client.NewClient(pc.Spec.Endpoint)}, nil
}

type external struct {
	client *client.Client
}

func (e *external) Disconnect(_ context.Context) error { return nil }

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.Network)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotNetwork)
	}

	exists, pool, err := e.client.NetworkExists(cr.Spec.ForProvider.NetworkKey)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errObserve)
	}

	if !exists {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	cr.Status.AtProvider.TotalIPs = pool.Total
	cr.Status.AtProvider.AssignedIPs = pool.Assigned + pool.Pending
	cr.Status.AtProvider.AvailableIPs = pool.Available

	meta.SetExternalName(cr, cr.Spec.ForProvider.NetworkKey)
	cr.SetConditions(xpv1.Available())

	// Check if desired IPs match what exists (for update detection).
	desiredIPs := computeDesiredIPs(cr)
	upToDate := len(desiredIPs) == 0 || pool.Total >= len(desiredIPs)

	return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: upToDate}, nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.Network)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotNetwork)
	}

	fp := cr.Spec.ForProvider

	// CIDR mode
	if fp.CIDR != "" {
		if err := e.client.CreateNetworkFromCIDR(fp.CIDR, fp.Reserved); err != nil {
			return managed.ExternalCreation{}, errors.Wrap(err, errCreate)
		}
		meta.SetExternalName(cr, fp.NetworkKey)
		return managed.ExternalCreation{}, nil
	}

	// Flat IPs or range mode
	ips := computeDesiredIPs(cr)
	if len(ips) == 0 {
		return managed.ExternalCreation{}, errors.New("no IPs specified: provide ips, ipFrom/ipTo, or cidr")
	}

	if err := e.client.CreateNetwork(fp.NetworkKey, ips); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreate)
	}

	meta.SetExternalName(cr, fp.NetworkKey)
	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.Network)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotNetwork)
	}

	// Get existing IPs to find what needs to be added.
	entries, err := e.client.GetNetworkIPs(cr.Spec.ForProvider.NetworkKey)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdate)
	}

	existingDigits := make(map[string]bool)
	for _, entry := range entries {
		existingDigits[entry.Digit] = true
	}

	desiredIPs := computeDesiredIPs(cr)
	var missing []string
	for _, ip := range desiredIPs {
		if !existingDigits[ip] {
			missing = append(missing, ip)
		}
	}

	if len(missing) > 0 {
		if err := e.client.AddIPs(cr.Spec.ForProvider.NetworkKey, missing); err != nil {
			return managed.ExternalUpdate{}, errors.Wrap(err, errUpdate)
		}
	}

	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.Network)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotNetwork)
	}

	if err := e.client.DeleteNetwork(cr.Spec.ForProvider.NetworkKey); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDelete)
	}

	return managed.ExternalDelete{}, nil
}

// computeDesiredIPs returns the list of IP suffixes from spec.
func computeDesiredIPs(cr *v1alpha1.Network) []string {
	fp := cr.Spec.ForProvider

	// Flat list
	if len(fp.IPs) > 0 {
		return fp.IPs
	}

	// Range mode
	if fp.IPFrom > 0 && fp.IPTo > 0 && fp.IPTo >= fp.IPFrom {
		var ips []string
		for i := fp.IPFrom; i <= fp.IPTo; i++ {
			ips = append(ips, fmt.Sprintf("%d", i))
		}
		return ips
	}

	return nil
}
