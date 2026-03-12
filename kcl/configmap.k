"""
ConfigMap for clusterbook
"""

import labels

config = labels.config

configMap = {
    apiVersion: "v1"
    kind: "ConfigMap"
    metadata: {
        name: "{}-config".format(config.name)
        namespace: config.namespace
        labels: labels.commonLabels
    }
    data: {
        LOAD_CONFIG_FROM: config.loadConfigFrom
        CONFIG_LOCATION: config.configLocation
        CONFIG_NAME: config.configName
        SERVER_PORT: config.serverPort
        HTTP_PORT: str(config.httpPort)
        PDNS_ENABLED: str(config.pdnsEnabled).lower()
        PDNS_URL: config.pdnsURL
        PDNS_TOKEN: config.pdnsToken
        PDNS_ZONE: config.pdnsZone
    }
}
