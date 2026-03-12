"""
Main entry point for clusterbook Kubernetes deployment
Imports all resources and exports manifests
"""

import namespace
import serviceaccount
import configmap
import deploy
import service
import rbac
import httproute

# Export all manifests (filter out None values)
manifests = [
    m for m in [
        namespace.namespace
        serviceaccount.serviceAccount
        configmap.configMap
        rbac.role
        rbac.roleBinding
        deploy.deployment
        service.serviceGrpc
        service.serviceHttp
        httproute.httproute
    ] if m
]
