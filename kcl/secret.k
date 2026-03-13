"""
Secret for clusterbook PowerDNS credentials
"""

import labels

config = labels.config

pdnsSecret = {
    apiVersion: "v1"
    kind: "Secret"
    metadata: {
        name: "{}-pdns".format(config.name)
        namespace: config.namespace
        labels: labels.commonLabels
    }
    type: "Opaque"
    stringData: {
        PDNS_TOKEN: config.pdnsToken
    }
} if config.pdnsEnabled else None
