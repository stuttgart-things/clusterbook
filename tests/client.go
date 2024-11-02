package main

import (
	"context"
	"fmt"
	"log"
	"time"

	ipservice "github.com/stuttgart-things/clusterbook/ipservice"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	//nolint
	// conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	// if err != nil {
	// 	log.Fatalf("did not connect: %v", err)
	// }
	// defer conn.Close()

	// c := ipservice.NewIpServiceClient(conn)

	// ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	// defer cancel()

	// // Testen der GetIpAddressRange-Methode
	// ipReq := &ipservice.IpRequest{
	// 	CountIpAddresses: 2,
	// 	NetworkKey:       "10.31.104",
	// }

	// ipRes, err := c.GetIpAddressRange(ctx, ipReq)
	// if err != nil {
	// 	log.Fatalf("could not get IP address range: %v", err)
	// }

	// fmt.Println(ipRes)

	// log.Printf("IP Address Range: %s", ipRes.IpAddressRange)

	// Testen der SetClusterInfo-Methode
	// clusterReq := &ipservice.ClusterRequest{
	// 	IpAddressRange: ipRes.IpAddressRange,
	// 	ClusterName:    "miami",
	// }

	// clusterRes, err := c.SetClusterInfo(ctx, clusterReq)
	// if err != nil {
	// 	log.Fatalf("could not set cluster info: %v", err)
	// }

	// log.Printf("Cluster Status: %s", clusterRes.Status)
	GetIps(2, "10.31.104")
	SetIpStatus("10.31.103.4", "sandiego")

}

func GetIps(countIps int32, networkKey string) {

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
		CountIpAddresses: countIps,
		NetworkKey:       networkKey,
	}

	ipRes, err := c.GetIpAddressRange(ctx, ipReq)
	if err != nil {
		log.Fatalf("could not get IP address range: %v", err)
	}

	fmt.Println(ipRes)

	log.Printf("IP Address Range: %s", ipRes.IpAddressRange)

}

func SetIpStatus(ips, clusterName string) {

	conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	c := ipservice.NewIpServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Testen der SetClusterInfo-Methode
	clusterReq := &ipservice.ClusterRequest{
		IpAddressRange: ips,
		ClusterName:    clusterName,
	}

	clusterRes, err := c.SetClusterInfo(ctx, clusterReq)
	if err != nil {
		log.Fatalf("could not set cluster info: %v", err)
	}

	log.Printf("Cluster Status: %s", clusterRes.Status)

}
