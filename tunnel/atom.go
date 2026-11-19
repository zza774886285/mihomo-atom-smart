package tunnel

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sync/atomic"
	"time"

	"github.com/metacubex/mihomo/component/fakeip"
	C "github.com/metacubex/mihomo/constant"
	D "github.com/miekg/dns"
)

type atomConfig struct {
	pool  *fakeip.Pool
	final string
	dns   string
}

var atomRuntime atomic.Pointer[atomConfig]

func ConfigureAtom(pool *fakeip.Pool, final, dns string) {
	if pool == nil {
		atomRuntime.Store(nil)
		return
	}
	atomRuntime.Store(&atomConfig{pool: pool, final: final, dns: dns})
}

func AtomReady() bool {
	atom := atomRuntime.Load()
	if atom == nil {
		return false
	}
	_, _, err := atom.pool.AtomBinding(netip.Addr{}, "")
	return err == nil
}

func MoveAtomBindings(group string) error {
	atom := atomRuntime.Load()
	if atom == nil {
		return errors.New("Atom FakeIP is disabled")
	}
	configMux.RLock()
	defer configMux.RUnlock()
	if _, exists := proxies[group]; exists {
		return fmt.Errorf("proxy group %q is still configured", group)
	}
	return atom.pool.MoveAtomBindings(group, atom.final)
}

func (a *atomConfig) applyFakeIP(metadata *C.Metadata, ip netip.Addr, domain string) error {
	host, outbound, err := a.pool.AtomBinding(ip, domain)
	if err != nil {
		return err
	}
	if host == "" {
		return fmt.Errorf("fake DNS record %s missing", ip)
	}
	if outbound == "" {
		outbound = a.final
	}
	metadata.Host = host
	metadata.DstIP = netip.Addr{}
	metadata.DNSMode = C.DNSFakeIP
	metadata.AtomFakeIP = ip
	metadata.SpecialProxy = outbound
	return nil
}

func (a *atomConfig) resolveMixed(metadata *C.Metadata) (bool, error) {
	if metadata.InName != "DEFAULT-MIXED" || metadata.Host == "" || metadata.DstIP.IsValid() || metadata.SpecialProxy != "" {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client := &D.Client{}
	for _, typ := range []uint16{D.TypeA, D.TypeAAAA} {
		query := new(D.Msg).SetQuestion(D.Fqdn(metadata.Host), typ)
		response, _, err := client.ExchangeContext(ctx, query, a.dns)
		if err == nil && response.Truncated {
			response, _, err = (&D.Client{Net: "tcp"}).ExchangeContext(ctx, query, a.dns)
		}
		if err != nil {
			return false, err
		}
		if response.Rcode != D.RcodeSuccess {
			return false, fmt.Errorf("AtomDNS returned %s for %s", D.RcodeToString[response.Rcode], metadata.Host)
		}
		var fake netip.Addr
		for _, answer := range response.Answer {
			var ip netip.Addr
			switch record := answer.(type) {
			case *D.A:
				ip, _ = netip.AddrFromSlice(record.A)
			case *D.AAAA:
				ip, _ = netip.AddrFromSlice(record.AAAA)
			}
			if !ip.IsValid() {
				continue
			}
			ip = ip.Unmap()
			if a.pool.IPNet().Contains(ip) {
				fake = ip
				continue
			}
			metadata.DstIP = ip
			metadata.DNSMode = C.DNSHosts
			return true, nil
		}
		if fake.IsValid() {
			err := a.applyFakeIP(metadata, netip.Addr{}, metadata.Host)
			metadata.AtomFakeIP = fake
			return false, err
		}
	}
	return false, fmt.Errorf("AtomDNS returned no addresses for %s", metadata.Host)
}
