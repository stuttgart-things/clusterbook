/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de
*/

package main

import (
	"context"
	"log"
	"net"
	"os"
	"path/filepath"
	"testing"

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
