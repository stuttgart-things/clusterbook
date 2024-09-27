/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de
*/

package main

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/stuttgart-things/clusterbook/internal"
	ipservice "github.com/stuttgart-things/clusterbook/ipservice"

	"google.golang.org/grpc"
)

type server struct {
	ipservice.UnimplementedIpServiceServer
}

func (s *server) GetIpAddressRange(ctx context.Context, req *ipservice.IpRequest) (*ipservice.IpResponse, error) {

	// LOAD CONFIG FROM HERE

	ipAddressRange := fmt.Sprintf("Generated IP range for networkKey %s with %d addresses", req.NetworkKey, req.CountIpAddresses)
	return &ipservice.IpResponse{IpAddressRange: ipAddressRange}, nil

}

func (s *server) SetClusterInfo(ctx context.Context, req *ipservice.ClusterRequest) (*ipservice.ClusterResponse, error) {
	// Beispiel-Implementierung: Setzen von Cluster-Informationen basierend auf ipAddressRange und clusterName
	status := fmt.Sprintf("Cluster %s set with IP range %s", req.ClusterName, req.IpAddressRange)
	return &ipservice.ClusterResponse{Status: status}, nil
}

func main() {

	// PRINT BANNER + VERSION INFO
	internal.PrintBanner()

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	ipservice.RegisterIpServiceServer(s, &server{})

	log.Printf("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
