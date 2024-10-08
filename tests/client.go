package main

import (
	"context"
	"log"
	"time"

	ipservice "github.com/stuttgart-things/clusterbook/ipservice"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	//nolint
	conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	c := ipservice.NewIpServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Testen der GetIpAddressRange-Methode
	ipReq := &ipservice.IpRequest{
		CountIpAddresses: 10,
		NetworkKey:       "exampleNetworkKey",
	}

	ipRes, err := c.GetIpAddressRange(ctx, ipReq)
	if err != nil {
		log.Fatalf("could not get IP address range: %v", err)
	}

	log.Printf("IP Address Range: %s", ipRes.IpAddressRange)

	// Testen der SetClusterInfo-Methode
	clusterReq := &ipservice.ClusterRequest{
		IpAddressRange: ipRes.IpAddressRange,
		ClusterName:    "exampleCluster",
	}

	clusterRes, err := c.SetClusterInfo(ctx, clusterReq)
	if err != nil {
		log.Fatalf("could not set cluster info: %v", err)
	}

	log.Printf("Cluster Status: %s", clusterRes.Status)
}
