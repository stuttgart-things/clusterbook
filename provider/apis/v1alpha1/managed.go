package v1alpha1

import (
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

// --- IPAssignment: implement resource.Managed ---

func (mg *IPAssignment) SetConditions(c ...xpv1.Condition)                        { mg.Status.SetConditions(c...) }
func (mg *IPAssignment) GetCondition(ct xpv1.ConditionType) xpv1.Condition        { return mg.Status.GetCondition(ct) }
func (mg *IPAssignment) GetProviderConfigReference() *xpv1.Reference              { return mg.Spec.ProviderConfigReference }
func (mg *IPAssignment) SetProviderConfigReference(r *xpv1.Reference)             { mg.Spec.ProviderConfigReference = r }
func (mg *IPAssignment) GetDeletionPolicy() xpv1.DeletionPolicy                   { return mg.Spec.DeletionPolicy }
func (mg *IPAssignment) SetDeletionPolicy(p xpv1.DeletionPolicy)                  { mg.Spec.DeletionPolicy = p }
func (mg *IPAssignment) GetManagementPolicies() xpv1.ManagementPolicies           { return mg.Spec.ManagementPolicies }
func (mg *IPAssignment) SetManagementPolicies(p xpv1.ManagementPolicies)          { mg.Spec.ManagementPolicies = p }
func (mg *IPAssignment) GetWriteConnectionSecretToReference() *xpv1.SecretReference { return mg.Spec.WriteConnectionSecretToReference }
func (mg *IPAssignment) SetWriteConnectionSecretToReference(r *xpv1.SecretReference) { mg.Spec.WriteConnectionSecretToReference = r }
func (mg *IPAssignment) GetPublishConnectionDetailsTo() *xpv1.PublishConnectionDetailsTo { return mg.Spec.PublishConnectionDetailsTo }
func (mg *IPAssignment) SetPublishConnectionDetailsTo(r *xpv1.PublishConnectionDetailsTo) { mg.Spec.PublishConnectionDetailsTo = r }

// --- Network: implement resource.Managed ---

func (mg *Network) SetConditions(c ...xpv1.Condition)                        { mg.Status.SetConditions(c...) }
func (mg *Network) GetCondition(ct xpv1.ConditionType) xpv1.Condition        { return mg.Status.GetCondition(ct) }
func (mg *Network) GetProviderConfigReference() *xpv1.Reference              { return mg.Spec.ProviderConfigReference }
func (mg *Network) SetProviderConfigReference(r *xpv1.Reference)             { mg.Spec.ProviderConfigReference = r }
func (mg *Network) GetDeletionPolicy() xpv1.DeletionPolicy                   { return mg.Spec.DeletionPolicy }
func (mg *Network) SetDeletionPolicy(p xpv1.DeletionPolicy)                  { mg.Spec.DeletionPolicy = p }
func (mg *Network) GetManagementPolicies() xpv1.ManagementPolicies           { return mg.Spec.ManagementPolicies }
func (mg *Network) SetManagementPolicies(p xpv1.ManagementPolicies)          { mg.Spec.ManagementPolicies = p }
func (mg *Network) GetWriteConnectionSecretToReference() *xpv1.SecretReference { return mg.Spec.WriteConnectionSecretToReference }
func (mg *Network) SetWriteConnectionSecretToReference(r *xpv1.SecretReference) { mg.Spec.WriteConnectionSecretToReference = r }
func (mg *Network) GetPublishConnectionDetailsTo() *xpv1.PublishConnectionDetailsTo { return mg.Spec.PublishConnectionDetailsTo }
func (mg *Network) SetPublishConnectionDetailsTo(r *xpv1.PublishConnectionDetailsTo) { mg.Spec.PublishConnectionDetailsTo = r }

// --- ProviderConfig: implement resource.ProviderConfig ---

func (p *ProviderConfig) SetConditions(c ...xpv1.Condition)                 { p.Status.SetConditions(c...) }
func (p *ProviderConfig) GetCondition(ct xpv1.ConditionType) xpv1.Condition { return p.Status.GetCondition(ct) }
func (p *ProviderConfig) SetUsers(i int64)                                   { p.Status.Users = i }
func (p *ProviderConfig) GetUsers() int64                                    { return p.Status.Users }

// --- ProviderConfigUsage: implement resource.ProviderConfigUsage ---

func (p *ProviderConfigUsage) GetProviderConfigReference() xpv1.Reference { return p.ProviderConfigUsage.ProviderConfigReference }
func (p *ProviderConfigUsage) SetProviderConfigReference(r xpv1.Reference) { p.ProviderConfigUsage.ProviderConfigReference = r }
func (p *ProviderConfigUsage) SetResourceReference(r xpv1.TypedReference) { p.ProviderConfigUsage.ResourceReference = r }
func (p *ProviderConfigUsage) GetResourceReference() xpv1.TypedReference  { return p.ProviderConfigUsage.ResourceReference }

// --- ManagedList implementations ---

// GetItems returns the list of managed IPAssignment resources.
func (l *IPAssignmentList) GetItems() []IPAssignment { return l.Items }

// GetItems returns the list of managed Network resources.
func (l *NetworkList) GetItems() []Network { return l.Items }

// GetItems returns the list of ProviderConfigUsage resources.
func (l *ProviderConfigUsageList) GetItems() []ProviderConfigUsage { return l.Items }
