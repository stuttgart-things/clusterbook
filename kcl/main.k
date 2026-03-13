"""
Main entry point for clusterbook Kubernetes deployment
Imports all resources and exports manifests
"""

import manifests as k8s_manifests
import crd
import namespace
import serviceaccount
import configmap
import secret
import deploy
import service
import rbac
import httproute

# Export all manifests as a YAML stream (filter out None values)
_manifests = [
    m for m in [
        crd.networkConfigCRD
        namespace.namespace
        serviceaccount.serviceAccount
        configmap.configMap
        secret.pdnsSecret
        rbac.role
        rbac.roleBinding
        deploy.deployment
        service.serviceGrpc
        service.serviceHttp
        httproute.httproute
    ] if m
]

k8s_manifests.yaml_stream(_manifests)
