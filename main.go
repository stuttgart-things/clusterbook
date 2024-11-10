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
	logger         = pterm.DefaultLogger.WithLevel(pterm.LogLevelTrace)
	loadConfigFrom = os.Getenv("LOAD_CONFIG_FROM")
	configFilePath = os.Getenv("CONFIG_FILE_PATH")
	// ipList         map[string]internal.IPs
)

func (s *server) GetIpAddressRange(ctx context.Context, req *ipservice.IpRequest) (*ipservice.IpResponse, error) {

	logger.Info("LOAD CONFIG FROM", logger.Args("", loadConfigFrom))
	logger.Info("CONFIG FILE PATH", logger.Args("", configFilePath))
	logger.Info("COUNT IPs", logger.Args("", req.CountIpAddresses))
	logger.Info("NETWORK KEY", logger.Args("", req.NetworkKey))

	// switch loadConfigFrom {
	// case "cr":
	// 	// READ NetworkConfig FROM CR
	// 	retrievedConfig, err := internal.GetNetworkConfig("networks-labul-2", "default")
	// 	if err != nil {
	// 		log.Fatalf("Failed to get NetworkConfig: %v", err)
	// 	}
	// 	fmt.Println("NETWORKS FROM CR:", retrievedConfig.Spec.Networks)
	// 	ipList = internal.ConvertFromCRFormat(retrievedConfig.Spec.Networks)
	// 	fmt.Println("NETWORKS CONVERT TO IPLIST FORMAT:", ipList)
	// case "disk":
	// 	// READ NetworkConfig FROM STATIC YAML FILE
	// 	ipList = internal.LoadProfile(loadConfigFrom, configFilePath)
	// 	fmt.Println("NETWORKS FROM STATIC YAML FILE:", ipList)
	// default:
	// 	log.Fatalf("INVALID LOAD_CONFIG_FROM VALUE: %s", loadConfigFrom)
	// }

	// READ NetworkConfig FROM CR
	// retrievedConfig, err := internal.GetNetworkConfig("networks-labul-2", "default")
	// if err != nil {
	// 	log.Fatalf("Failed to get NetworkConfig: %v", err)
	// }
	// fmt.Println("NETWORKS FROM CR:", retrievedConfig.Spec.Networks)
	// ipList := internal.ConvertFromCRFormat(retrievedConfig.Spec.Networks)
	// fmt.Println("NETWORKS CONVERT TO IPLIST FORMAT:", ipList)

	// READ NetworkConfig FROM STATIC YAML FILE
	ipList := internal.LoadProfile(loadConfigFrom, configFilePath)
	fmt.Println("NETWORKS FROM STATC YAML FILE:", ipList)

	// CONVERT TO CR FORMAT
	// result := internal.ConvertToCRFormat(ipList)
	// fmt.Println("NETWORKS CONVERT TO CR FORMAT:", result)

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

	//var bla = "hello"

	logger.Info("LOAD CONFIG FROM", logger.Args("", loadConfigFrom))
	logger.Info("CONFIG FILE PATH", logger.Args("", configFilePath))

	// LOAD EXISTING YAML FILE
	ipList := internal.LoadProfile(loadConfigFrom, configFilePath)

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

	// SAVE YAML FILE
	internal.SaveYAMLToDisk(ipList, configFilePath)

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
