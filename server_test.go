package main

import (
	"context"
	"log"
	"testing"

	"net"

	ipservice "github.com/stuttgart-things/clusterbook/ipservice"
	"google.golang.org/grpc"
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

func TestGetIpAddressRange(t *testing.T) {
	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(bufDialer), grpc.WithInsecure())
	if err != nil {
		t.Fatalf("Failed to dial bufnet: %v", err)
	}
	defer conn.Close()
	client := ipservice.NewIpServiceClient(conn)

	req := &ipservice.IpRequest{
		CountIpAddresses: 10,
		NetworkKey:       "exampleNetworkKey",
	}

	resp, err := client.GetIpAddressRange(ctx, req)
	if err != nil {
		t.Fatalf("GetIpAddressRange failed: %v", err)
	}

	expected := "Generated IP range for networkKey exampleNetworkKey with 10 addresses"
	if resp.IpAddressRange != expected {
		t.Errorf("Expected %s, got %s", expected, resp.IpAddressRange)
	}
}

func TestSetClusterInfo(t *testing.T) {
	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(bufDialer), grpc.WithInsecure())
	if err != nil {
		t.Fatalf("Failed to dial bufnet: %v", err)
	}
	defer conn.Close()
	client := ipservice.NewIpServiceClient(conn)

	req := &ipservice.ClusterRequest{
		IpAddressRange: "Generated IP range for networkKey exampleNetworkKey with 10 addresses",
		ClusterName:    "exampleCluster",
	}

	resp, err := client.SetClusterInfo(ctx, req)
	if err != nil {
		t.Fatalf("SetClusterInfo failed: %v", err)
	}

	expected := "Cluster exampleCluster set with IP range Generated IP range for networkKey exampleNetworkKey with 10 addresses"
	if resp.Status != expected {
		t.Errorf("Expected %s, got %s", expected, resp.Status)
	}
}
