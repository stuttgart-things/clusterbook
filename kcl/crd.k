"""
CustomResourceDefinition for NetworkConfig
"""

networkConfigCRD = {
    apiVersion: "apiextensions.k8s.io/v1"
    kind: "CustomResourceDefinition"
    metadata: {
        name: "networkconfigs.github.stuttgart-things.com"
    }
    spec: {
        group: "github.stuttgart-things.com"
        names: {
            kind: "NetworkConfig"
            plural: "networkconfigs"
            singular: "networkconfig"
            shortNames: ["nc"]
        }
        scope: "Namespaced"
        versions: [
            {
                name: "v1"
                served: True
                storage: True
                schema: {
                    openAPIV3Schema: {
                        type: "object"
                        properties: {
                            spec: {
                                type: "object"
                                properties: {
                                    networks: {
                                        type: "object"
                                        additionalProperties: {
                                            type: "array"
                                            items: {
                                                type: "string"
                                            }
                                        }
                                        description: "Network pools keyed by subnet prefix, values are IP entries (e.g. '5:ASSIGNED:cluster-name')"
                                    }
                                }
                                required: ["networks"]
                            }
                        }
                    }
                }
            }
        ]
    }
}
