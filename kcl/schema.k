"""
Schema definition for clusterbook deployment configuration
"""

schema ClusterBook:
    """Configuration for clusterbook Kubernetes deployment"""

    # Metadata
    name: str = "clusterbook"
    namespace: str = "clusterbook"

    # Image
    image: str = "ghcr.io/stuttgart-things/clusterbook:v1.11.0"
    imagePullPolicy: str = "Always"

    # Replicas and resources
    replicas: int = 1
    cpuRequest: str = "50m"
    cpuLimit: str = "100m"
    memoryRequest: str = "64Mi"
    memoryLimit: str = "128Mi"

    # Ports
    grpcPort: int = 50051
    httpPort: int = 8080
    grpcServicePort: int = 80
    httpServicePort: int = 8080

    # Config
    loadConfigFrom: str = "cr"
    configLocation: str = "clusterbook"
    configName: str = "networks-labul"
    serverPort: str = "50051"

    # Service
    serviceType: str = "ClusterIP"

    # HTTPRoute (Gateway API)
    httpRouteEnabled: bool = False
    httpRouteParentRefName: str = ""
    httpRouteParentRefNamespace: str = ""
    httpRouteHostname: str = ""
    httpRouteAnnotations: {str:str} = {}

    # PowerDNS integration
    pdnsEnabled: bool = False
    pdnsURL: str = ""
    pdnsToken: str = ""
    pdnsZone: str = ""

    # DD-WRT integration
    ddwrtEnabled: bool = False
    ddwrtHost: str = ""
    ddwrtUser: str = ""
    ddwrtPassword: str = ""
    ddwrtZone: str = ""

    # RBAC
    networkConfigApiGroup: str = "github.stuttgart-things.com"

    # Labels and annotations
    labels: {str:str} = {}
    annotations: {str:str} = {}

    check:
        replicas >= 0, "replicas must be >= 0"
        grpcPort >= 1 and grpcPort <= 65535, "grpcPort must be valid"
        httpPort >= 1 and httpPort <= 65535, "httpPort must be valid"
        name != "", "name must not be empty"
        namespace != "", "namespace must not be empty"
