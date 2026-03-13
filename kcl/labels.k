"""
Common labels and configuration for clusterbook resources
"""

import schema

# Get command-line options
_image = option("config.image")
_namespace = option("config.namespace")
_loadConfigFrom = option("config.loadConfigFrom")
_configLocation = option("config.configLocation")
_configName = option("config.configName")
_serverPort = option("config.serverPort")
_httpRouteEnabled = option("config.httpRouteEnabled")
_httpRouteParentRefName = option("config.httpRouteParentRefName")
_httpRouteParentRefNamespace = option("config.httpRouteParentRefNamespace")
_httpRouteHostname = option("config.httpRouteHostname")
_httpRouteAnnotations = option("config.httpRouteAnnotations") or {}
_pdnsEnabled = option("config.pdnsEnabled")
_pdnsURL = option("config.pdnsURL")
_pdnsToken = option("config.pdnsToken")
_pdnsZone = option("config.pdnsZone")

# Config instance with command-line overrides
config: schema.ClusterBook = schema.ClusterBook {
    if _image:
        image = _image
    if _namespace:
        namespace = _namespace
    if _loadConfigFrom:
        loadConfigFrom = _loadConfigFrom
    if _configLocation:
        configLocation = _configLocation
    if _configName:
        configName = _configName
    if _serverPort:
        serverPort = _serverPort
    if _httpRouteEnabled:
        httpRouteEnabled = _httpRouteEnabled in [True, "True", "true"]
    if _httpRouteParentRefName:
        httpRouteParentRefName = _httpRouteParentRefName
    if _httpRouteParentRefNamespace:
        httpRouteParentRefNamespace = _httpRouteParentRefNamespace
    if _httpRouteHostname:
        httpRouteHostname = _httpRouteHostname
    if _httpRouteAnnotations:
        httpRouteAnnotations = _httpRouteAnnotations
    if _pdnsEnabled:
        pdnsEnabled = _pdnsEnabled in [True, "True", "true"]
    if _pdnsURL:
        pdnsURL = _pdnsURL
    if _pdnsToken:
        pdnsToken = _pdnsToken
    if _pdnsZone:
        pdnsZone = _pdnsZone
}

# Common labels applied to all resources
commonLabels = {
    "app.kubernetes.io/name": config.name
    "app.kubernetes.io/instance": config.name
    "app.kubernetes.io/component": "ipam"
    "app.kubernetes.io/part-of": "clusterbook"
    **config.labels
}

# Selector labels for pods
selectorLabels = {
    "app.kubernetes.io/name": config.name
    "app.kubernetes.io/instance": config.name
}
