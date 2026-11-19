package fakeip

import (
	"fmt"
	"net/netip"
	"strings"
)

func (p *Pool) AtomBinding(ip netip.Addr, host string) (string, string, error) {
	return p.atom.LookupBinding(ip, host)
}

func (p *Pool) MoveAtomBindings(group, final string) error {
	return p.atom.MoveBindings(group, final)
}

func (p *Pool) SetAtomOutbounds(outbounds map[string]bool) {
	p.atomOutbounds = outbounds
}

func (p *Pool) IsAtom() bool { return p != nil && p.atom != nil }

func (p *Pool) LookupDNS(host string, binding *string) (netip.Addr, error) {
	if p.atom == nil {
		return p.Lookup(host), nil
	}
	if binding != nil && *binding != "" && !p.atomOutbounds[*binding] {
		return netip.Addr{}, fmt.Errorf("unknown Atom FakeIP outbound %q", *binding)
	}
	return p.atom.Assign(strings.ToLower(host), p.first, p.last, binding)
}
