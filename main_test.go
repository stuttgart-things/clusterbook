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
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stuttgart-things/clusterbook/internal"
	ipservice "github.com/stuttgart-things/clusterbook/ipservice"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

var lis *bufconn.Listener

func init() {
	lis = bufconn.Listen(bufSize)
	s := grpc.NewServer()
	ipservice.RegisterIpServiceServer(s, &server{})
	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("Server exited with error: %v", err)
		}
	}()
}

func bufDialer(context.Context, string) (net.Conn, error) {
	return lis.Dial()
}

// func TestGetIpAddressRange(t *testing.T) {
// 	ctx := context.Background()

// 	//lint:ignore
// 	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(bufDialer), grpc.WithTransportCredentials(insecure.NewCredentials()))

// 	if err != nil {
// 		t.Fatalf("Failed to dial bufnet: %v", err)
// 	}
// 	defer conn.Close()
// 	client := ipservice.NewIpServiceClient(conn)

// 	req := &ipservice.IpRequest{
// 		CountIpAddresses: 2,
// 		NetworkKey:       "10.31.103",
// 	}

// 	resp, err := client.GetIpAddressRange(ctx, req)
// 	if err != nil {
// 		t.Fatalf("GetIpAddressRange failed: %v", err)
// 	}

// 	expected := "Generated IP range for networkKey exampleNetworkKey with 10 addresses"
// 	if resp.IpAddressRange != expected {
// 		t.Errorf("Expected %s, got %s", expected, resp.IpAddressRange)
// 	}
// }

func useDiskConfig(t *testing.T, yaml string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	prevFrom, prevLoc, prevName := loadConfigFrom, configLocation, configName
	loadConfigFrom, configLocation, configName = "disk", dir, "config.yaml"
	t.Cleanup(func() {
		loadConfigFrom, configLocation, configName = prevFrom, prevLoc, prevName
	})
}

const grpcTestConfigYAML = `
10.31.103:
  "5":
    status: "ASSIGNED:DNS"
    cluster: mycluster
  "6":
    status: ""
    cluster: ""
`

// Before issue #196 these paths called log.Fatalf, so an ordinary request for
// more addresses than the pool holds took the server down for every caller. A
// regression here would exit the test binary rather than fail an assertion.
func TestGetIpAddressRange_ReturnsStatusInsteadOfExiting(t *testing.T) {
	useDiskConfig(t, grpcTestConfigYAML)

	tests := []struct {
		name string
		req  *ipservice.IpRequest
		want codes.Code
	}{
		{"not enough addresses", &ipservice.IpRequest{CountIpAddresses: 2, NetworkKey: "10.31.103"}, codes.ResourceExhausted},
		{"unknown network", &ipservice.IpRequest{CountIpAddresses: 1, NetworkKey: "10.99.99"}, codes.NotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := (&server{}).GetIpAddressRange(context.Background(), tt.req)
			if got := status.Code(err); got != tt.want {
				t.Errorf("code = %v (err %v), want %v", got, err, tt.want)
			}
		})
	}
}

func TestGetIpAddressRange_SkipsAssignedDNS(t *testing.T) {
	useDiskConfig(t, grpcTestConfigYAML)

	resp, err := (&server{}).GetIpAddressRange(context.Background(),
		&ipservice.IpRequest{CountIpAddresses: 1, NetworkKey: "10.31.103"})
	if err != nil {
		t.Fatalf("GetIpAddressRange: %v", err)
	}
	if resp.IpAddressRange != "10.31.103.6" {
		t.Errorf("IpAddressRange = %q, want the only free address 10.31.103.6", resp.IpAddressRange)
	}
}

// An IP outside every configured network used to write into a nil map and panic,
// and unparsable input hit log.Fatalf — either one ended the server. A regression
// here would crash the test binary rather than fail an assertion.
func TestSetClusterInfo_RejectsBadInputWithoutWriting(t *testing.T) {
	useDiskConfig(t, grpcTestConfigYAML)
	path := filepath.Join(configLocation, configName)

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		ips  string
		want codes.Code
	}{
		// The valid first IP proves nothing is saved when a later one fails.
		{"unknown network", "10.31.103.6;10.99.99.1", codes.NotFound},
		{"not an ip", "10.31.103.6;bogus", codes.InvalidArgument},
		{"empty range", "", codes.InvalidArgument},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := (&server{}).SetClusterInfo(context.Background(),
				&ipservice.ClusterRequest{IpAddressRange: tt.ips, ClusterName: "probe", Status: "ASSIGNED"})
			if got := status.Code(err); got != tt.want {
				t.Fatalf("code = %v (err %v), want %v", got, err, tt.want)
			}

			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) {
				t.Errorf("config was rewritten despite the error:\n%s", after)
			}
		})
	}
}

func TestSetClusterInfo_PersistsAssignment(t *testing.T) {
	useDiskConfig(t, grpcTestConfigYAML)

	if _, err := (&server{}).SetClusterInfo(context.Background(),
		&ipservice.ClusterRequest{IpAddressRange: "10.31.103.6", ClusterName: "probe", Status: "ASSIGNED"}); err != nil {
		t.Fatalf("SetClusterInfo: %v", err)
	}

	ipList, err := internal.LoadProfile(loadConfigFrom, configLocation, configName)
	if err != nil {
		t.Fatal(err)
	}
	if got := ipList["10.31.103"]["6"]; got.Status != "ASSIGNED" || got.Cluster != "probe" {
		t.Errorf("entry .6 = %+v, want ASSIGNED/probe", got)
	}
}

// Issue #200: in cr mode the save error was printed and the RPC still reported
// success; disk mode could not even see the error.
func TestSetClusterInfo_SaveFailureReturnsInternal(t *testing.T) {
	useDiskConfig(t, grpcTestConfigYAML)

	if err := os.Chmod(configLocation, 0o555); err != nil {
		t.Fatal(err)
	}
	dir := configLocation
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if probe, err := os.CreateTemp(configLocation, "probe-*"); err == nil {
		_ = probe.Close()
		_ = os.Remove(probe.Name())
		t.Skip("directory permissions are not enforced here (running as root?)")
	}

	resp, err := (&server{}).SetClusterInfo(context.Background(),
		&ipservice.ClusterRequest{IpAddressRange: "10.31.103.6", ClusterName: "probe", Status: "ASSIGNED"})
	if got := status.Code(err); got != codes.Internal {
		t.Fatalf("code = %v (resp %v, err %v), want Internal", got, resp, err)
	}
}

const grpcPoolYAML = `
10.31.103:
  "5":
    status: "ASSIGNED:DNS"
    cluster: mycluster
  "6":
    status: ""
    cluster: ""
  "7":
    status: ""
    cluster: ""
  "8":
    status: ""
    cluster: ""
`

// Issue #199: GetIpAddressRange + SetClusterInfo is two calls with nothing held in
// between. ReserveIpAddresses records what it hands out in the same write.
func TestReserveIpAddresses_RecordsWhatItReturns(t *testing.T) {
	useDiskConfig(t, grpcPoolYAML)
	srv := &server{}

	resp, err := srv.ReserveIpAddresses(context.Background(),
		&ipservice.ReserveRequest{CountIpAddresses: 2, NetworkKey: "10.31.103", ClusterName: "grpc-a"})
	if err != nil {
		t.Fatalf("ReserveIpAddresses: %v", err)
	}
	ips := strings.Split(resp.IpAddressRange, ";")
	if len(ips) != 2 {
		t.Fatalf("IpAddressRange = %q, want 2 addresses", resp.IpAddressRange)
	}

	ipList, err := internal.LoadProfile(loadConfigFrom, configLocation, configName)
	if err != nil {
		t.Fatal(err)
	}
	for _, ip := range ips {
		digit := ip[strings.LastIndex(ip, ".")+1:]
		if got := ipList["10.31.103"][digit]; got.Status != "ASSIGNED" || got.Cluster != "grpc-a" {
			t.Errorf("%s in ledger = %+v, want ASSIGNED/grpc-a", ip, got)
		}
	}

	// One address is left; asking for two must not hand out the recorded ones.
	if _, err := srv.ReserveIpAddresses(context.Background(),
		&ipservice.ReserveRequest{CountIpAddresses: 2, NetworkKey: "10.31.103", ClusterName: "grpc-b"}); status.Code(err) != codes.ResourceExhausted {
		t.Errorf("second reserve: code = %v, want ResourceExhausted", status.Code(err))
	}
}

func TestReserveIpAddresses_Concurrent(t *testing.T) {
	useDiskConfig(t, grpcPoolYAML)
	srv := &server{}

	const n = 3 // exactly the free addresses
	got := make([]string, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resp, err := srv.ReserveIpAddresses(context.Background(),
				&ipservice.ReserveRequest{CountIpAddresses: 1, NetworkKey: "10.31.103", ClusterName: fmt.Sprintf("c%d", i)})
			if err != nil {
				t.Errorf("reserve %d: %v", i, err)
				return
			}
			got[i] = resp.IpAddressRange
		}(i)
	}
	wg.Wait()

	seen := map[string]bool{}
	for _, ip := range got {
		if ip != "" && seen[ip] {
			t.Errorf("%s handed out twice: %v", ip, got)
		}
		seen[ip] = true
	}
}

func TestReserveIpAddresses_RejectsBadRequests(t *testing.T) {
	useDiskConfig(t, grpcPoolYAML)

	tests := []struct {
		name string
		req  *ipservice.ReserveRequest
		want codes.Code
	}{
		{"no cluster", &ipservice.ReserveRequest{CountIpAddresses: 1, NetworkKey: "10.31.103"}, codes.InvalidArgument},
		{"zero count", &ipservice.ReserveRequest{NetworkKey: "10.31.103", ClusterName: "c"}, codes.InvalidArgument},
		{"dns status without a record", &ipservice.ReserveRequest{CountIpAddresses: 1, NetworkKey: "10.31.103", ClusterName: "c", Status: "ASSIGNED:DNS"}, codes.InvalidArgument},
		{"unknown network", &ipservice.ReserveRequest{CountIpAddresses: 1, NetworkKey: "10.99.99", ClusterName: "c"}, codes.NotFound},
		{"too many", &ipservice.ReserveRequest{CountIpAddresses: 4, NetworkKey: "10.31.103", ClusterName: "c"}, codes.ResourceExhausted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := (&server{}).ReserveIpAddresses(context.Background(), tt.req)
			if got := status.Code(err); got != tt.want {
				t.Errorf("code = %v (err %v), want %v", got, err, tt.want)
			}
		})
	}
}
