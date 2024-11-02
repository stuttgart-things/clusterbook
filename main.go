/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de
*/

package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	"github.com/pterm/pterm"
	"github.com/stuttgart-things/clusterbook/internal"
	ipservice "github.com/stuttgart-things/clusterbook/ipservice"

	"google.golang.org/grpc"
)

type server struct {
	ipservice.UnimplementedIpServiceServer
}

var (
	logger = pterm.DefaultLogger.WithLevel(pterm.LogLevelTrace)
)

func (s *server) GetIpAddressRange(ctx context.Context, req *ipservice.IpRequest) (*ipservice.IpResponse, error) {

	// CONFIG FILE PATH FROM ENV
	loadConfigFrom := os.Getenv("LOAD_CONFIG_FROM")
	configFilePath := os.Getenv("CONFIG_FILE_PATH")
	logger.Info("LOAD CONFIG FROM", logger.Args("", loadConfigFrom))
	logger.Info("CONFIG FILE PATH", logger.Args("", configFilePath))
	logger.Info("COUNT IPs", logger.Args("", req.CountIpAddresses))
	logger.Info("NETWORK KEY", logger.Args("", req.NetworkKey))

	// LOAD CONFIG FROM HERE
	ipList := internal.LoadProfile(loadConfigFrom, configFilePath)
	fmt.Println(ipList)

	availableAddresses, err := internal.GenerateIPs(ipList, int(req.CountIpAddresses), req.NetworkKey)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	fmt.Println(availableAddresses)

	ips := strings.Join(availableAddresses, ";")

	//ipAddressRange := fmt.Sprintf("Generated IP range for networkKey %s with %d addresses", req.NetworkKey, req.CountIpAddresses)
	return &ipservice.IpResponse{IpAddressRange: ips}, nil

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
