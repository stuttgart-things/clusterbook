"""
Services for clusterbook (gRPC + HTTP)
"""

import labels

config = labels.config

# gRPC service (primary, for programmatic access)
serviceGrpc = {
    apiVersion: "v1"
    kind: "Service"
    metadata: {
        name: config.name
        namespace: config.namespace
        labels: labels.commonLabels
    }
    spec: {
        type: config.serviceType
        ports: [
            {
                name: "grpc"
                port: config.grpcServicePort
                targetPort: "grpc"
                protocol: "TCP"
            }
        ]
        selector: labels.selectorLabels
    }
}

# HTTP service (for HTMX frontend and REST API)
serviceHttp = {
    apiVersion: "v1"
    kind: "Service"
    metadata: {
        name: "{}-http".format(config.name)
        namespace: config.namespace
        labels: labels.commonLabels
    }
    spec: {
        type: config.serviceType
        ports: [
            {
                name: "http"
                port: config.httpServicePort
                targetPort: "http"
                protocol: "TCP"
            }
        ]
        selector: labels.selectorLabels
    }
}
