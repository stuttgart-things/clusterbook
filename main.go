/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de
*/

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/pterm/pterm"
	"github.com/stuttgart-things/clusterbook/internal"
	ipservice "github.com/stuttgart-things/clusterbook/ipservice"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	port    = ":50051"
	webPort = "8080"
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
	httpPort       = os.Getenv("HTTP_PORT")
	pdnsEnabled    = os.Getenv("PDNS_ENABLED")
	pdnsURL        = os.Getenv("PDNS_URL")
	pdnsToken      = os.Getenv("PDNS_TOKEN")
	pdnsZone       = os.Getenv("PDNS_ZONE")
	ddwrtEnabled   = os.Getenv("DDWRT_ENABLED")
	ddwrtHost      = os.Getenv("DDWRT_HOST")
	ddwrtUser      = os.Getenv("DDWRT_USER")
	ddwrtPassword  = os.Getenv("DDWRT_PASSWORD")
	ddwrtZone      = os.Getenv("DDWRT_ZONE")
	reclaimerInt   = os.Getenv("RECLAIMER_INTERVAL")
)

// defaultReclaimerInterval is used when RECLAIMER_INTERVAL is empty.
// Set RECLAIMER_INTERVAL=0 (or a negative duration) to disable.
const defaultReclaimerInterval = 60 * time.Second

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
	ipList, err := internal.LoadProfile(loadConfigFrom, configLocation, configName)
	if err != nil {
		logger.Error("FAILED TO LOAD CONFIG", logger.Args("err", err.Error()))
		return nil, err
	}
	fmt.Println("NETWORKS FROM STATC YAML FILE:", ipList)

	availableAddresses, err := internal.GenerateIPs(ipList, int(req.CountIpAddresses), req.NetworkKey)
	if err != nil {
		logger.Error("FAILED TO GENERATE IPS", logger.Args("network", req.NetworkKey, "err", err.Error()))
		return nil, generateIPsStatus(err)
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

	// LOAD EXISTING CONFIG — as one ledger write, so no other writer interleaves (issue #199)
	tx := internal.BeginLedgerWrite(loadConfigFrom, configLocation, configName)
	defer tx.End()

	ipList, err := tx.Load()
	if err != nil {
		logger.Error("FAILED TO LOAD CONFIG", logger.Args("err", err.Error()))
		return nil, status.Error(codes.Internal, err.Error())
	}

	// GET IPS FROM REQUEST
	ips := strings.Split(req.IpAddressRange, ";")

	// LOOP OVER ips — returning early is safe: nothing is saved until the loop is done
	for _, ip := range ips {
		// TRUNCATE IP
		ipKey, err := internal.TruncateIP(ip)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		ipDigit, err := internal.GetLastIPDigit(ip)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		// An unconfigured network has a nil map; writing into it panics and,
		// with no recovery interceptor, takes the whole server down.
		if _, ok := ipList[ipKey]; !ok {
			return nil, status.Errorf(codes.NotFound, "NETWORK %s IS NOT CONFIGURED", ipKey)
		}

		entry := ipList[ipKey][ipDigit]

		if entry.Status == "" {
			logger.Info("IP WAS NOT SET", logger.Args("", ipKey+"."+ipDigit))
		}
		entry.Status = req.Status // Use the status from the request
		entry.Cluster = req.ClusterName

		ipList[ipKey][ipDigit] = entry // Reassign the modified struct back to the map
		logger.Info("IP WAS ASSIGNED", logger.Args("", ipKey+"."+ipDigit))
	}

	fmt.Println(ipList)
	result := fmt.Sprintf("CLUSTER %s SET WITH IP RANGE %s AND STATUS %s", req.ClusterName, req.IpAddressRange, req.Status)

	// CONVERT THE STATUS TO UPPERCASE
	result = strings.ToUpper(result)

	// SAVE — a failed write must not be reported as a success (issue #200)
	if err := tx.Save(ipList); err != nil {
		logger.Error("FAILED TO SAVE CONFIG", logger.Args("err", err.Error()))
		return nil, saveStatus(err)
	}

	return &ipservice.ClusterResponse{Status: result}, nil
}

// ReserveIpAddresses picks free addresses and records them in one ledger write.
// GetIpAddressRange followed by SetClusterInfo is two calls with nothing held in
// between, so concurrent callers received the same addresses (issue #199).
func (s *server) ReserveIpAddresses(ctx context.Context, req *ipservice.ReserveRequest) (*ipservice.IpResponse, error) {
	if req.ClusterName == "" {
		return nil, status.Error(codes.InvalidArgument, "clusterName is required")
	}
	if req.CountIpAddresses < 1 {
		return nil, status.Error(codes.InvalidArgument, "countIpAddresses must be at least 1")
	}

	entryStatus := req.Status
	if entryStatus == "" {
		entryStatus = "ASSIGNED"
	}
	if strings.HasSuffix(entryStatus, ":DNS") {
		// A ":DNS" status without a record would claim something untrue.
		return nil, status.Error(codes.InvalidArgument, "DNS records are not managed over gRPC; reserve over HTTP with create_dns")
	}

	tx := internal.BeginLedgerWrite(loadConfigFrom, configLocation, configName)
	defer tx.End()

	ipList, err := tx.Load()
	if err != nil {
		logger.Error("FAILED TO LOAD CONFIG", logger.Args("err", err.Error()))
		return nil, status.Error(codes.Internal, err.Error())
	}

	picked, err := internal.GenerateIPs(ipList, int(req.CountIpAddresses), req.NetworkKey)
	if err != nil {
		return nil, generateIPsStatus(err)
	}
	if len(picked) == 0 {
		return nil, status.Error(codes.ResourceExhausted, internal.ErrNotEnoughAddresses.Error())
	}

	for _, ip := range picked {
		digit, err := internal.GetLastIPDigit(ip)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		ipList[req.NetworkKey][digit] = internal.IPInfo{Status: entryStatus, Cluster: req.ClusterName}
	}

	if err := tx.Save(ipList); err != nil {
		logger.Error("FAILED TO SAVE CONFIG", logger.Args("err", err.Error()))
		return nil, saveStatus(err)
	}

	logger.Info("RESERVED IPS", logger.Args("cluster", req.ClusterName, "ips", picked))
	return &ipservice.IpResponse{IpAddressRange: strings.Join(picked, ";")}, nil
}

// generateIPsStatus maps a GenerateIPs error to a gRPC status. A too-small pool or
// an unknown network is an answer to the request, not a reason to stop the server.
func generateIPsStatus(err error) error {
	switch {
	case errors.Is(err, internal.ErrNetworkNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, internal.ErrNotEnoughAddresses):
		return status.Error(codes.ResourceExhausted, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

// saveStatus maps a ledger save error: a concurrent change is Aborted, which
// tells the client to retry the whole call; anything else is Internal.
func saveStatus(err error) error {
	if errors.Is(err, internal.ErrLedgerConflict) {
		return status.Error(codes.Aborted, err.Error())
	}
	return status.Error(codes.Internal, err.Error())
}

func main() {

	// PRINT BANNER + VERSION INFO
	internal.PrintBanner()

	if serverPort == "" {
		serverPort = port
	} else {
		serverPort = ":" + os.Getenv("SERVER_PORT")
	}

	if httpPort == "" {
		httpPort = webPort
	}

	// INIT DNS PROVIDER CLIENTS (nil if not enabled)
	pdns := internal.NewPDNSClient(pdnsEnabled, pdnsURL, pdnsToken, pdnsZone)
	ddwrt := internal.NewDDWRTClient(ddwrtEnabled, ddwrtHost, ddwrtUser, ddwrtPassword, ddwrtZone)

	// START HTTP/HTMX SERVER IN BACKGROUND
	go internal.StartWebServer(httpPort, loadConfigFrom, configLocation, configName, pdns, ddwrt)

	// START LEASE RECLAIMER IN BACKGROUND
	interval := defaultReclaimerInterval
	if reclaimerInt != "" {
		if d, err := time.ParseDuration(reclaimerInt); err == nil {
			interval = d
		} else {
			logger.Warn("INVALID RECLAIMER_INTERVAL, USING DEFAULT", logger.Args("value", reclaimerInt, "default", interval.String()))
		}
	}
	go internal.StartReclaimer(context.Background(), interval, loadConfigFrom, configLocation, configName, pdns, ddwrt)

	lis, err := net.Listen("tcp", serverPort)
	if err != nil {
		log.Fatalf("FAILED TO LISTEN: %v", err)
	}

	s := grpc.NewServer()
	ipservice.RegisterIpServiceServer(s, &server{})

	log.Printf("GRPC SERVER LISTENING AT %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("FAILED TO SERVE: %v", err)
	}
}
