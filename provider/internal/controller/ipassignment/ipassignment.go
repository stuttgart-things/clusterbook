package ipassignment

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	errNotIPAssignment = "managed resource is not an IPAssignment"
	errGetPC           = "cannot get ProviderConfig"
	errObserve         = "cannot observe IPAssignment"
	errCreate          = "cannot create IPAssignment"
	errUpdate          = "cannot update IPAssignment"
	errDelete          = "cannot delete IPAssignment"
)

// Setup adds the IPAssignment controller to the manager.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.IPAssignmentGroupKind)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		For(&v1alpha1.IPAssignment{}).
		Complete(managed.NewReconciler(mgr,
			resource.ManagedKind(v1alpha1.IPAssignmentGroupVersionKind),
			managed.WithExternalConnecter(&connector{kube: mgr.GetClient()}),
			managed.WithLogger(o.Logger.WithValues("controller", name)),
			managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		))
}

type connector struct {
	kube k8sclient.Client
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.IPAssignment)
	if !ok {
		return nil, errors.New(errNotIPAssignment)
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
	cr, ok := mg.(*v1alpha1.IPAssignment)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotIPAssignment)
	}

	// If we haven't assigned IPs yet, the resource doesn't exist externally.
	if len(cr.Status.AtProvider.IPAddresses) == 0 {
		// Check if this cluster already has IPs in the network (adopt partial creates).
		entries, err := e.client.GetNetworkIPs(cr.Spec.ForProvider.NetworkKey)
		if err != nil {
			return managed.ExternalObservation{}, errors.Wrap(err, errObserve)
		}

		var existing []client.IPEntry
		for _, entry := range entries {
			if entry.Cluster == cr.Spec.ForProvider.Cluster && strings.HasPrefix(entry.Status, "ASSIGNED") {
				existing = append(existing, entry)
			}
		}

		if len(existing) > 0 && len(existing) >= cr.Spec.ForProvider.CountIPs {
			adopted := existing[:cr.Spec.ForProvider.CountIPs]
			ips := make([]string, len(adopted))
			for i, e := range adopted {
				ips[i] = e.IP
			}
			cr.Status.AtProvider.IPAddresses = ips
			cr.Status.AtProvider.IPAddress = ips[0]
			cr.Status.AtProvider.FQDN = adopted[0].FQDN
			cr.Status.AtProvider.LeaseExpiresAt = minLeaseExpiresAt(adopted)
			meta.SetExternalName(cr, cr.Spec.ForProvider.Cluster+"/"+cr.Spec.ForProvider.NetworkKey)
			cr.SetConditions(xpv1.Available())
			return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, nil
		}

		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	// Verify IPs are still assigned to the correct cluster.
	entries, err := e.client.GetNetworkIPs(cr.Spec.ForProvider.NetworkKey)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errObserve)
	}

	entryMap := make(map[string]client.IPEntry)
	for _, entry := range entries {
		entryMap[entry.IP] = entry
	}

	observed := make([]client.IPEntry, 0, len(cr.Status.AtProvider.IPAddresses))
	for _, ip := range cr.Status.AtProvider.IPAddresses {
		entry, exists := entryMap[ip]
		if !exists || entry.Cluster != cr.Spec.ForProvider.Cluster {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		observed = append(observed, entry)
	}

	// Refresh status from observed state.
	cr.Status.AtProvider.FQDN = observed[0].FQDN
	cr.Status.AtProvider.LeaseExpiresAt = minLeaseExpiresAt(observed)

	// Check if status/cluster needs update.
	upToDate := true
	for _, entry := range observed {
		expectedStatus := cr.Spec.ForProvider.Status
		if cr.Spec.ForProvider.CreateDNS {
			expectedStatus += ":DNS"
		}
		if entry.Status != expectedStatus || entry.Cluster != cr.Spec.ForProvider.Cluster {
			upToDate = false
			break
		}
	}

	// Heartbeat renewal: when a lease duration is requested, consider the
	// resource out-of-date once the remaining TTL drops below half the desired
	// duration, so Update() renews it.
	if upToDate && cr.Spec.ForProvider.LeaseDurationSeconds > 0 {
		threshold := time.Now().Unix() + cr.Spec.ForProvider.LeaseDurationSeconds/2
		for _, entry := range observed {
			if entry.LeaseExpiresAt == 0 || entry.LeaseExpiresAt < threshold {
				upToDate = false
				break
			}
		}
	}

	cr.SetConditions(xpv1.Available())
	return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: upToDate}, nil
}

// minLeaseExpiresAt returns the earliest lease expiry across entries, or 0
// if any entry has no lease (meaning the set as a whole has no lease horizon).
func minLeaseExpiresAt(entries []client.IPEntry) int64 {
	var min int64
	for i, e := range entries {
		if e.LeaseExpiresAt == 0 {
			return 0
		}
		if i == 0 || e.LeaseExpiresAt < min {
			min = e.LeaseExpiresAt
		}
	}
	return min
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.IPAssignment)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotIPAssignment)
	}

	countIPs := cr.Spec.ForProvider.CountIPs
	if countIPs == 0 {
		countIPs = 1
	}

	available, err := e.client.FindAvailableIPs(cr.Spec.ForProvider.NetworkKey, countIPs)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreate)
	}

	status := cr.Spec.ForProvider.Status
	if status == "" {
		status = "ASSIGNED"
	}

	var assignedIPs []string
	for _, entry := range available {
		if err := e.client.AssignIP(
			cr.Spec.ForProvider.NetworkKey,
			entry.IP,
			cr.Spec.ForProvider.Cluster,
			status,
			cr.Spec.ForProvider.CreateDNS,
			cr.Spec.ForProvider.LeaseDurationSeconds,
		); err != nil {
			return managed.ExternalCreation{}, errors.Wrap(err, fmt.Sprintf("assigning IP %s", entry.IP))
		}
		assignedIPs = append(assignedIPs, entry.IP)
	}

	cr.Status.AtProvider.IPAddresses = assignedIPs
	if len(assignedIPs) > 0 {
		cr.Status.AtProvider.IPAddress = assignedIPs[0]
	}
	if cr.Spec.ForProvider.LeaseDurationSeconds > 0 {
		cr.Status.AtProvider.LeaseExpiresAt = time.Now().Unix() + cr.Spec.ForProvider.LeaseDurationSeconds
	}

	meta.SetExternalName(cr, cr.Spec.ForProvider.Cluster+"/"+cr.Spec.ForProvider.NetworkKey)

	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.IPAssignment)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotIPAssignment)
	}

	status := cr.Spec.ForProvider.Status
	if status == "" {
		status = "ASSIGNED"
	}

	for _, ip := range cr.Status.AtProvider.IPAddresses {
		parts := strings.Split(ip, ".")
		if len(parts) < 4 {
			continue
		}
		ipDigit := parts[len(parts)-1]

		if err := e.client.EditIP(
			cr.Spec.ForProvider.NetworkKey,
			ipDigit,
			cr.Spec.ForProvider.Cluster,
			status,
			cr.Spec.ForProvider.CreateDNS,
		); err != nil {
			return managed.ExternalUpdate{}, errors.Wrap(err, errUpdate)
		}

		if cr.Spec.ForProvider.LeaseDurationSeconds > 0 {
			if err := e.client.RenewLease(cr.Spec.ForProvider.NetworkKey, ip, cr.Spec.ForProvider.LeaseDurationSeconds); err != nil {
				return managed.ExternalUpdate{}, errors.Wrap(err, errUpdate)
			}
		}
	}

	if cr.Spec.ForProvider.LeaseDurationSeconds > 0 {
		cr.Status.AtProvider.LeaseExpiresAt = time.Now().Unix() + cr.Spec.ForProvider.LeaseDurationSeconds
	}

	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.IPAssignment)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotIPAssignment)
	}

	for _, ip := range cr.Status.AtProvider.IPAddresses {
		if err := e.client.ReleaseIP(cr.Spec.ForProvider.NetworkKey, ip); err != nil {
			return managed.ExternalDelete{}, errors.Wrap(err, errDelete)
		}
	}

	return managed.ExternalDelete{}, nil
}
