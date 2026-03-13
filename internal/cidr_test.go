/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de
*/

package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCIDRToNetworks_Slash24(t *testing.T) {
	networks, err := CIDRToNetworks("10.31.103.0/24", nil)
	require.NoError(t, err)

	assert.Len(t, networks, 1)
	assert.Contains(t, networks, "10.31.103")

	ips := networks["10.31.103"]
	// /24 = 256 addresses, minus network (.0) and broadcast (.255) = 254 usable
	assert.Len(t, ips, 254)

	// Verify .0 and .255 are excluded
	assert.NotContains(t, ips, "0")
	assert.NotContains(t, ips, "255")

	// Verify .1 and .254 are included
	assert.Contains(t, ips, "1")
	assert.Contains(t, ips, "254")
}

func TestCIDRToNetworks_Slash28(t *testing.T) {
	networks, err := CIDRToNetworks("10.31.103.0/28", nil)
	require.NoError(t, err)

	assert.Len(t, networks, 1)
	ips := networks["10.31.103"]
	// /28 = 16 addresses, minus network (.0) and broadcast (.15) = 14 usable
	assert.Len(t, ips, 14)
	assert.Contains(t, ips, "1")
	assert.Contains(t, ips, "14")
	assert.NotContains(t, ips, "0")
	assert.NotContains(t, ips, "15")
}

func TestCIDRToNetworks_Slash30(t *testing.T) {
	networks, err := CIDRToNetworks("10.31.103.0/30", nil)
	require.NoError(t, err)

	ips := networks["10.31.103"]
	// /30 = 4 addresses, minus network and broadcast = 2 usable
	assert.Len(t, ips, 2)
	assert.Contains(t, ips, "1")
	assert.Contains(t, ips, "2")
}

func TestCIDRToNetworks_WithReserved(t *testing.T) {
	reserved := []string{"1", "2"}
	networks, err := CIDRToNetworks("10.31.103.0/24", reserved)
	require.NoError(t, err)

	ips := networks["10.31.103"]
	// 254 usable minus 2 reserved = 252
	assert.Len(t, ips, 252)
	assert.NotContains(t, ips, "1")
	assert.NotContains(t, ips, "2")
	assert.Contains(t, ips, "3")
}

func TestCIDRToNetworks_CrossSlash24Boundary(t *testing.T) {
	networks, err := CIDRToNetworks("10.31.103.0/23", nil)
	require.NoError(t, err)

	// /23 spans two /24 blocks
	assert.Len(t, networks, 2)
	assert.Contains(t, networks, "10.31.102")
	assert.Contains(t, networks, "10.31.103")

	// First block: 10.31.102.1-254 (network is 10.31.102.0, broadcast is 10.31.103.255)
	// All 255 addresses in 10.31.102 except .0 (network address) = 255
	assert.Len(t, networks["10.31.102"], 255)
	// Second block: all 255 except .255 (broadcast)
	assert.Len(t, networks["10.31.103"], 255)
}

func TestCIDRToNetworks_InvalidCIDR(t *testing.T) {
	_, err := CIDRToNetworks("not-a-cidr", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid CIDR")
}

func TestCIDRToNetworks_IPv6Rejected(t *testing.T) {
	_, err := CIDRToNetworks("::1/128", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "IPv4")
}

func TestCIDRToNetworks_NonZeroHost(t *testing.T) {
	// CIDR with non-zero host bits - net.ParseCIDR normalizes to network address
	networks, err := CIDRToNetworks("10.31.103.50/28", nil)
	require.NoError(t, err)

	ips := networks["10.31.103"]
	// 10.31.103.50/28 -> network is 10.31.103.48, broadcast is 10.31.103.63
	// Usable: 49-62 = 14 hosts
	assert.Len(t, ips, 14)
	assert.Contains(t, ips, "49")
	assert.Contains(t, ips, "62")
	assert.NotContains(t, ips, "48")
	assert.NotContains(t, ips, "63")
}

func TestCIDRToIPList(t *testing.T) {
	ips, err := CIDRToIPList("10.31.103.0/30", nil)
	require.NoError(t, err)

	assert.Len(t, ips, 2)
	assert.Contains(t, ips, "10.31.103.1")
	assert.Contains(t, ips, "10.31.103.2")
}

func TestValidateCIDR(t *testing.T) {
	assert.NoError(t, ValidateCIDR("10.0.0.0/8"))
	assert.NoError(t, ValidateCIDR("192.168.1.0/24"))
	assert.Error(t, ValidateCIDR("invalid"))
	assert.Error(t, ValidateCIDR("::1/128"))
}

func TestCIDRNetworkKey(t *testing.T) {
	key, err := CIDRNetworkKey("10.31.103.0/24")
	require.NoError(t, err)
	assert.Equal(t, "10.31.103", key)

	key, err = CIDRNetworkKey("192.168.1.0/24")
	require.NoError(t, err)
	assert.Equal(t, "192.168.1", key)
}
