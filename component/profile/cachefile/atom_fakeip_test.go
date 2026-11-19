package cachefile

import (
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

func TestAtomFakeIPMappingAndBindingLifecycle(t *testing.T) {
	C.SetHomeDir(t.TempDir())
	store := AtomFakeIP{Prefix: netip.MustParsePrefix("198.18.0.0/29")}
	first := netip.MustParseAddr("198.18.0.4")
	last := netip.MustParseAddr("198.18.0.7")
	group := "g-00000001"
	assertBinding := func(ip netip.Addr, wantHost, wantGroup string) {
		t.Helper()
		host, binding, err := store.LookupBinding(ip, "")
		require.NoError(t, err)
		require.Equal(t, wantHost, host)
		require.Equal(t, wantGroup, binding)
	}

	ip, err := store.Assign("first.invalid", first, last, &group)
	require.NoError(t, err)
	require.Equal(t, first, ip)
	assertBinding(ip, "first.invalid", group)
	clear := ""
	same, err := store.Assign("first.invalid", first, last, &clear)
	require.NoError(t, err)
	require.Equal(t, ip, same)
	assertBinding(ip, "first.invalid", "")
	_, err = store.Assign("first.invalid", first, last, &group)
	require.NoError(t, err)

	var assigned []netip.Addr
	for _, name := range []string{"second.invalid", "third.invalid", "last.invalid", "recycled.invalid"} {
		got, err := store.Assign(name, first, last, nil)
		require.NoError(t, err)
		assigned = append(assigned, got)
	}
	// the whole range is used, last address included, before the cursor wraps
	require.Equal(t, []netip.Addr{first.Next(), first.Next().Next(), last, first}, assigned)
	assertBinding(ip, "recycled.invalid", "")
	_, stale, err := store.LookupBinding(netip.Addr{}, "first.invalid")
	require.NoError(t, err)
	require.Empty(t, stale)

	ip, err = store.Assign("migrate.invalid", first, last, &group)
	require.NoError(t, err)
	require.NoError(t, store.MoveBindings(group, "final"))
	assertBinding(ip, "migrate.invalid", "final")
	_, err = store.Assign("migrate.invalid", first, last, &group)
	require.Error(t, err)
	require.NoError(t, store.Flush())
	assertBinding(ip, "", "")
}

func TestAtomFakeIPClearRetired(t *testing.T) {
	C.SetHomeDir(t.TempDir())
	store := AtomFakeIP{Prefix: netip.MustParsePrefix("198.18.0.0/29")}
	first := netip.MustParseAddr("198.18.0.4")
	last := netip.MustParseAddr("198.18.0.7")
	group := "g-00000001"
	_, err := store.Assign("a.invalid", first, last, &group)
	require.NoError(t, err)
	require.NoError(t, store.MoveBindings(group, "final"))
	_, err = store.Assign("b.invalid", first, last, &group)
	require.Error(t, err)
	require.NoError(t, store.ClearRetired([]string{group}))
	_, err = store.Assign("b.invalid", first, last, &group)
	require.NoError(t, err)
}
