package cachefile

import (
	"bytes"
	"errors"
	"net/netip"
	"strings"

	"github.com/metacubex/bbolt"
)

// AtomFakeIP keeps a pool's addresses, bindings and allocation cursor in the
// same transaction. The prefix scopes the data so a config check or a failed
// range change cannot destroy the live pool.
type AtomFakeIP struct {
	Prefix netip.Prefix
}

func (a AtomFakeIP) transaction(write bool, run func(*bbolt.Bucket) error) error {
	db := Cache().DB
	if db == nil {
		return errors.New("Atom FakeIP cache unavailable")
	}
	name := []byte("atom-fakeip:" + a.Prefix.Masked().String())
	if write {
		return db.Update(func(tx *bbolt.Tx) error {
			bucket, err := tx.CreateBucketIfNotExists(name)
			if err != nil {
				return err
			}
			return run(bucket)
		})
	}
	return db.View(func(tx *bbolt.Tx) error {
		return run(tx.Bucket(name))
	})
}

func (a AtomFakeIP) Assign(host string, first, last netip.Addr, binding *string) (ip netip.Addr, err error) {
	hostKey := []byte("h:" + host)
	bindingKey := []byte("b:" + host)
	err = a.transaction(false, func(bucket *bbolt.Bucket) error {
		if bucket != nil {
			ip, _ = netip.AddrFromSlice(bucket.Get(hostKey))
			if binding != nil && (string(bucket.Get(bindingKey)) != *binding || bucket.Get([]byte("retired:"+*binding)) != nil) {
				ip = netip.Addr{}
			}
		}
		return nil
	})
	if err != nil || ip.IsValid() {
		return
	}
	err = a.transaction(true, func(bucket *bbolt.Bucket) error {
		if binding != nil && bucket.Get([]byte("retired:"+*binding)) != nil {
			return errors.New("Atom FakeIP outbound has been removed")
		}
		if existing, ok := netip.AddrFromSlice(bucket.Get(hostKey)); ok {
			ip = existing
		} else {
			cursor := []byte("cursor")
			offset, ok := netip.AddrFromSlice(bucket.Get(cursor))
			ip = first
			if ok && !last.Less(offset.Next()) {
				ip = offset.Next()
			}
			addressKey := []byte("i:" + ip.String())
			if former := string(bucket.Get(addressKey)); former != "" {
				if err := bucket.Delete([]byte("h:" + former)); err != nil {
					return err
				}
				if err := bucket.Delete([]byte("b:" + former)); err != nil {
					return err
				}
			}
			if err := bucket.Put(addressKey, []byte(host)); err != nil {
				return err
			}
			if err := bucket.Put(hostKey, ip.AsSlice()); err != nil {
				return err
			}
			if err := bucket.Put(cursor, ip.AsSlice()); err != nil {
				return err
			}
		}
		if binding == nil || string(bucket.Get(bindingKey)) == *binding {
			return nil
		}
		if *binding == "" {
			return bucket.Delete(bindingKey)
		}
		return bucket.Put(bindingKey, []byte(*binding))
	})
	return
}

func (a AtomFakeIP) LookupBinding(ip netip.Addr, domain string) (host, outbound string, err error) {
	err = a.transaction(false, func(bucket *bbolt.Bucket) error {
		host = domain
		if bucket == nil {
			return nil
		}
		if ip.IsValid() {
			host = string(bucket.Get([]byte("i:" + ip.String())))
		}
		if host != "" {
			outbound = string(bucket.Get([]byte("b:" + strings.ToLower(host))))
		}
		return nil
	})
	return
}

func (a AtomFakeIP) MoveBindings(group, final string) error {
	return a.transaction(true, func(bucket *bbolt.Bucket) error {
		prefix := []byte("b:")
		cursor := bucket.Cursor()
		var keys [][]byte
		for key, value := cursor.Seek(prefix); bytes.HasPrefix(key, prefix); key, value = cursor.Next() {
			if string(value) == group {
				keys = append(keys, append([]byte(nil), key...))
			}
		}
		for _, key := range keys {
			if err := bucket.Put(key, []byte(final)); err != nil {
				return err
			}
		}
		return bucket.Put([]byte("retired:"+group), []byte{1})
	})
}

// ClearRetired drops the retired marks of groups that exist in the applied configuration again.
func (a AtomFakeIP) ClearRetired(groups []string) error {
	return a.transaction(true, func(bucket *bbolt.Bucket) error {
		for _, group := range groups {
			if err := bucket.Delete([]byte("retired:" + group)); err != nil {
				return err
			}
		}
		return nil
	})
}

func (a AtomFakeIP) Flush() error {
	db := Cache().DB
	if db == nil {
		return errors.New("Atom FakeIP cache unavailable")
	}
	name := []byte("atom-fakeip:" + a.Prefix.Masked().String())
	return db.Update(func(tx *bbolt.Tx) error {
		if tx.Bucket(name) == nil {
			return nil
		}
		return tx.DeleteBucket(name)
	})
}
