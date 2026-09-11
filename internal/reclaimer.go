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

// ReclaimExpiredLeases clears expired entries in-place and returns them. It does
// not touch DNS: records are removed only after the cleared ledger has been saved,
// so a failed save cannot leave a still-assigned address without its record
// (issue #200).
func ReclaimExpiredLeases(ipList map[string]IPs, now time.Time) []ExpiredLease {
	expired := FindExpiredLeases(ipList, now)
	for _, e := range expired {
		entry := ipList[e.NetworkKey][e.IPDigit]
		entry.Status = ""
		entry.Cluster = ""
		entry.LeaseExpiresAt = 0
		ipList[e.NetworkKey][e.IPDigit] = entry
	}
	return expired
}

// removeReclaimedDNS deletes the DNS record of every reclaimed lease that had one.
func removeReclaimedDNS(reclaimed []ExpiredLease, pdns *PDNSClient, ddwrt *DDWRTClient) {
	for _, e := range reclaimed {
		if !e.HadDNS || e.Cluster == "" {
			continue
		}
		var dns dnsResult
		dns.remove(pdns, ddwrt, e.Cluster)
		if dns.failed() {
			logger := pterm.DefaultLogger.WithLevel(pterm.LogLevelTrace)
			logger.Warn("reclaimer dns delete failed", logger.Args("cluster", e.Cluster, "err", dns.message()))
		}
	}
}

// reclaimOnce runs one reclaimer cycle: load, clear expired leases, save, and only
// then remove their DNS records. When the save fails nothing counts as reclaimed.
func reclaimOnce(loadFrom, configLoc, configNm string, now time.Time, pdns *PDNSClient, ddwrt *DDWRTClient) ([]ExpiredLease, error) {
	ipList, err := LoadProfile(loadFrom, configLoc, configNm)
	if err != nil {
		return nil, err
	}

	reclaimed := ReclaimExpiredLeases(ipList, now)
	if len(reclaimed) == 0 {
		return nil, nil
	}

	if err := SaveConfig(ipList, loadFrom, configLoc, configNm); err != nil {
		return nil, err
	}

	removeReclaimedDNS(reclaimed, pdns, ddwrt)
	return reclaimed, nil
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
			reclaimed, err := reclaimOnce(loadFrom, configLoc, configNm, time.Now(), pdns, ddwrt)
			if err != nil {
				logger.Warn("lease reclaimer: cycle failed, nothing reclaimed", logger.Args("err", err.Error()))
				continue
			}
			for _, e := range reclaimed {
				logger.Info("lease reclaimed", logger.Args("ip", e.NetworkKey+"."+e.IPDigit, "cluster", e.Cluster, "dns_cleanup", e.HadDNS))
			}
		}
	}
}
