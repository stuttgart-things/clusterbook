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

const (
	port = ":50051"
)

type server struct {
	ipservice.UnimplementedIpServiceServer
}

var (
	logger         = pterm.DefaultLogger.WithLevel(pterm.LogLevelTrace)
	loadConfigFrom = os.Getenv("LOAD_CONFIG_FROM")
	configName     = os.Getenv("CONFIG_NAME")
	configLocation = os.Getenv("CONFIG_LOCATION")
	serverPort     = os.Getenv("SERVER_PORT")
)

func (s *server) GetIpAddressRange(ctx context.Context, req *ipservice.IpRequest) (*ipservice.IpResponse, error) {
	logger.Info("LOAD CONFIG FROM", logger.Args("", loadConfigFrom))
	logger.Info("CONFIG NAME", logger.Args("", configName))
	logger.Info("CONFIG LOCATION", logger.Args("", configLocation))
	logger.Info("COUNT IPs", logger.Args("", req.CountIpAddresses))
	logger.Info("NETWORK KEY", logger.Args("", req.NetworkKey))

	if serverPort == "" {
		serverPort = port
	} else {
		serverPort = os.Getenv("SERVER_PORT")
	}

	// READ NetworkConfig FROM STATIC YAML FILE
	ipList := internal.LoadProfile(loadConfigFrom, configLocation, configName)
	fmt.Println("NETWORKS FROM STATC YAML FILE:", ipList)

	availableAddresses, err := internal.GenerateIPs(ipList, int(req.CountIpAddresses), req.NetworkKey)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	logger.Info("AVAILABLE ADDRESSES", logger.Args("", availableAddresses))

	if len(availableAddresses) == 0 {
		return &ipservice.IpResponse{IpAddressRange: "NO AVAILABLE ADDRESSES"}, nil
	} else {
		ips := strings.Join(availableAddresses, ";")
		return &ipservice.IpResponse{IpAddressRange: ips}, nil
	}
}

func (s *server) SetClusterInfo(ctx context.Context, req *ipservice.ClusterRequest) (*ipservice.ClusterResponse, error) {
	logger.Info("LOAD CONFIG FROM", logger.Args("", loadConfigFrom))
	logger.Info("CONFIG FILE PATH", logger.Args("", configLocation+"/"+configName))

	// LOAD EXISTING YAML FILE
	ipList := internal.LoadProfile(loadConfigFrom, configLocation, configName)

	// GET IPS FROM REQUEST
	ips := strings.Split(req.IpAddressRange, ";")

	// LOOP OVEER ips
	for _, ip := range ips {

		// TRUNCATE IP
		ipKey, err := internal.TruncateIP(ip)
		if err != nil {
			log.Fatalf("error: %v", err)
		}

		ipDigit, err := internal.GetLastIPDigit(ip)
		if err != nil {
			log.Fatalf("error: %v", err)
		}

		entry := ipList[ipKey][ipDigit]

		if entry.Status == "" {
			logger.Info("IP WAS NOT SET", logger.Args("", ipKey+"."+ipDigit))
		}
		entry.Status = "ASSIGNED" // Modify the field
		entry.Cluster = req.ClusterName

		ipList[ipKey][ipDigit] = entry // Reassign the modified struct back to the map
		logger.Info("IP WAS ASSIGNED", logger.Args("", ipKey+"."+ipDigit))

	}

	fmt.Println(ipList)
	status := fmt.Sprintf("CLUSTER %s SET WITH IP RANGE %s", req.ClusterName, req.IpAddressRange)

	// SAVE YAML FILE
	switch loadConfigFrom {

	case "disk":
		internal.SaveYAMLToDisk(ipList, configLocation+"/"+configName)
	case "cr":
		ipListCR := internal.ConvertToCRFormat(ipList)
		err := internal.CreateOrUpdateNetworkConfig(ipListCR, configName, configLocation)
		fmt.Println(err)
	default:
		log.Fatalf("INVALID LOAD_CONFIG_FROM VALUE: %s", loadConfigFrom)
	}

	return &ipservice.ClusterResponse{Status: status}, nil
}

func main() {
	// PRINT BANNER + VERSION INFO
	internal.PrintBanner()

	if serverPort == "" {
		serverPort = port
	} else {
		serverPort = ":" + os.Getenv("SERVER_PORT")
	}

	lis, err := net.Listen("tcp", serverPort)
	if err != nil {
		log.Fatalf("FAILED TO LISTEN: %v", err)
	}

	s := grpc.NewServer()
	ipservice.RegisterIpServiceServer(s, &server{})

	log.Printf("SERVER LISTENING AT %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("FAILED TO SERVE: %v", err)
	}
}
