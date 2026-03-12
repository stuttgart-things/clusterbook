[package]
name = "deploy-clusterbook"
version = "0.1.0"
description = "KCL module for deploying clusterbook on Kubernetes"

[dependencies]
k8s = "1.31"

[profile]
entries = [
    "main.k"
]
