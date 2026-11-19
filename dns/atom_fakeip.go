package dns

import (
	"errors"
	"strings"

	D "github.com/miekg/dns"
)

const atomFakeIPOption = 65001

func atomBinding(msg *D.Msg) (*string, error) {
	opt := msg.IsEdns0()
	if opt == nil {
		return nil, nil
	}
	for _, option := range opt.Option {
		value, ok := option.(*D.EDNS0_LOCAL)
		if !ok || value.Code != atomFakeIPOption || len(value.Data) < 2 || value.Data[0] != 1 {
			continue
		}
		switch value.Data[1] {
		case 1:
			outbound := strings.TrimSpace(string(value.Data[2:]))
			if outbound == "" {
				return nil, errors.New("empty Atom FakeIP outbound")
			}
			switch outbound {
			case "direct":
				outbound = "DIRECT"
			case "block":
				outbound = "REJECT"
			}
			return &outbound, nil
		case 2:
			outbound := ""
			return &outbound, nil
		}
	}
	return nil, nil
}

func filterAtomOption(options []D.EDNS0) []D.EDNS0 {
	filtered := options[:0]
	for _, option := range options {
		if option.Option() != atomFakeIPOption {
			filtered = append(filtered, option)
		}
	}
	return filtered
}
