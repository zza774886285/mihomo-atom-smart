package dns

import (
	"testing"

	D "github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

func TestAtomFakeIPOption(t *testing.T) {
	query := new(D.Msg).SetQuestion("test.invalid.", D.TypeA).SetEdns0(1232, false)
	opt := query.IsEdns0()
	option := &D.EDNS0_LOCAL{Code: atomFakeIPOption, Data: []byte{1, 1}}
	opt.Option = append(opt.Option, option)
	option.Data = append(option.Data, []byte("g-00000001")...)
	binding, err := atomBinding(query)
	require.NoError(t, err)
	require.NotNil(t, binding)
	require.Equal(t, "g-00000001", *binding)

	option.Data = []byte{1, 2}
	binding, err = atomBinding(query)
	require.NoError(t, err)
	require.NotNil(t, binding)
	require.Empty(t, *binding)
	require.Empty(t, filterAtomOption(opt.Option))

	option.Data = []byte{2, 2}
	binding, err = atomBinding(query)
	require.NoError(t, err)
	require.Nil(t, binding)
}
