/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de
*/

package internal

import (
	"context"
	"strings"
	"time"

	"github.com/pterm/pterm"
)

// ExpiredLease identifies an IP entry whose lease has expired.
type ExpiredLease struct {
	NetworkKey string
	IPDigit    string
	Cluster    string
	HadDNS     bool
}

// FindExpiredLeases returns entries whose LeaseExpiresAt is non-zero and already in the past.
func FindExpiredLeases(ipList map[string]IPs, now time.Time) []ExpiredLease {
	var expired []ExpiredLease
	for networkKey, ips := range ipList {
		for digit, info := range ips {
			if info.LeaseExpiresAt == 0 {
				continue
			}
			if info.LeaseExpiresAt > now.Unix() {
				continue
			}
			expired = append(expired, ExpiredLease{
				NetworkKey: networkKey,
				IPDigit:    digit,
				Cluster:    info.Cluster,
				HadDNS:     strings.HasSuffix(info.Status, ":DNS"),
			})
		}
	}
	return expired
}

// ReclaimExpiredLeases clears expired entries in-place, invokes DNS cleanup for entries
// that had a DNS record, and returns the reclaimed leases.
func ReclaimExpiredLeases(ipList map[string]IPs, now time.Time, pdns *PDNSClient, ddwrt *DDWRTClient) []ExpiredLease {
	expired := FindExpiredLeases(ipList, now)
	for _, e := range expired {
		entry := ipList[e.NetworkKey][e.IPDigit]
		entry.Status = ""
		entry.Cluster = ""
		entry.LeaseExpiresAt = 0
		ipList[e.NetworkKey][e.IPDigit] = entry

		if e.HadDNS && e.Cluster != "" {
			if pdns != nil {
				pdns.DeleteRecord(e.Cluster)
			}
			if ddwrt != nil {
				if err := ddwrt.DeleteRecord(e.Cluster); err != nil {
					pterm.DefaultLogger.WithLevel(pterm.LogLevelTrace).Warn("reclaimer ddwrt delete failed", pterm.DefaultLogger.Args("cluster", e.Cluster, "err", err))
				}
			}
		}
	}
	return expired
}

// StartReclaimer runs a periodic loop that reclaims expired leases.
// Disabled if interval is <= 0. Blocks until ctx is cancelled.
func StartReclaimer(ctx context.Context, interval time.Duration, loadFrom, configLoc, configNm string, pdns *PDNSClient, ddwrt *DDWRTClient) {
	logger := pterm.DefaultLogger.WithLevel(pterm.LogLevelTrace)
	if interval <= 0 {
		logger.Info("lease reclaimer disabled")
		return
	}
	logger.Info("lease reclaimer started", logger.Args("interval", interval.String()))

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("lease reclaimer stopped")
			return
		case <-ticker.C:
			ipList := LoadProfile(loadFrom, configLoc, configNm)
			reclaimed := ReclaimExpiredLeases(ipList, time.Now(), pdns, ddwrt)
			if len(reclaimed) > 0 {
				saveConfig(ipList, loadFrom, configLoc, configNm)
				for _, e := range reclaimed {
					logger.Info("lease reclaimed", logger.Args("ip", e.NetworkKey+"."+e.IPDigit, "cluster", e.Cluster, "dns_cleanup", e.HadDNS))
				}
			}
		}
	}
}
