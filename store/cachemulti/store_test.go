package cachemulti_test

import (
	"fmt"
	"github.com/pokt-network/pocket-core/store/cachemulti"
	rand2 "github.com/tendermint/tendermint/libs/rand"
	"testing"

	"github.com/stretchr/testify/require"
	dbm "github.com/tendermint/tm-db"

	"github.com/pokt-network/pocket-core/store/types"
)

// Wrapper type for dbm.Db with implementation of KVStore
type DBAdapterStore struct {
	dbm.DB
}

// GetStoreType returns the type of the store.
func (DBAdapterStore) GetStoreType() types.StoreType {
	return types.StoreTypeDB
}

// CacheWrap cache wraps the underlying store.
func (dsa DBAdapterStore) CacheWrap() types.CacheWrap {
	return cachemulti.NewStore(dsa)
}

// dbm.DB implements KVStore so we can CacheKVStore it.
var _ types.KVStore = DBAdapterStore{}

func newCacheKVStore() types.CacheKVStore {
	mem := DBAdapterStore{DB: dbm.NewMemDB()}
	return cachemulti.NewStore(mem)
}

func keyFmt(i int) []byte { return bz(fmt.Sprintf("key%0.8d", i)) }
func valFmt(i int) []byte { return bz(fmt.Sprintf("value%0.8d", i)) }

func TestCacheKVStore(t *testing.T) {
	mem := DBAdapterStore{DB: dbm.NewMemDB()}
	st := cachemulti.NewStore(mem)

	g, _ := st.Get(keyFmt(1))
	require.Empty(t, g, "Expected `key1` to be empty")

	// put something in mem and in cache
	_ = mem.Set(keyFmt(1), valFmt(1))
	_ = st.Set(keyFmt(1), valFmt(1))
	g, _ = st.Get(keyFmt(1))
	require.Equal(t, valFmt(1), g)

	// update it in cache, shoudn't change mem
	_ = st.Set(keyFmt(1), valFmt(2))
	g, _ = st.Get(keyFmt(1))
	require.Equal(t, valFmt(2), g)
	mg, _ := mem.Get(keyFmt(1))
	require.Equal(t, valFmt(1), mg)

	// write it. should change mem
	st.Write()
	mg, _ = mem.Get(keyFmt(1))
	require.Equal(t, valFmt(2), mg)
	sg, _ := st.Get(keyFmt(1))
	require.Equal(t, valFmt(2), sg)

	// more writes and checks
	st.Write()
	st.Write()
	mg, _ = mem.Get(keyFmt(1))
	sg, _ = st.Get(keyFmt(1))
	require.Equal(t, valFmt(2), mg)
	require.Equal(t, valFmt(2), sg)

	// make a new one, check it
	st = cachemulti.NewStore(mem)
	sg, _ = st.Get(keyFmt(1))
	require.Equal(t, valFmt(2), sg)

	// make a new one and delete - should not be removed from mem
	st = cachemulti.NewStore(mem)
	_ = st.Delete(keyFmt(1))
	sg, _ = st.Get(keyFmt(1))
	require.Empty(t, sg)
	mg, _ = mem.Get(keyFmt(1))
	require.Equal(t, mg, valFmt(2))

	// Write. should now be removed from both
	st.Write()
	sg, _ = st.Get(keyFmt(1))
	mg, _ = mem.Get(keyFmt(1))
	require.Empty(t, sg, "Expected `key1` to be empty")
	require.Empty(t, mg, "Expected `key1` to be empty")
}

func TestCacheKVStoreNoNilSet(t *testing.T) {
	mem := DBAdapterStore{DB: dbm.NewMemDB()}
	st := cachemulti.NewStore(mem)
	require.Panics(t, func() { _ = st.Set([]byte("key"), nil) }, "setting a nil value should panic")
}

func TestCacheKVStoreNested(t *testing.T) {
	mem := DBAdapterStore{DB: dbm.NewMemDB()}
	st := cachemulti.NewStore(mem)

	// set. check its there on st and not on mem.
	_ = st.Set(keyFmt(1), valFmt(1))
	mg, _ := mem.Get(keyFmt(1))
	require.Empty(t, mg)
	sg, _ := st.Get(keyFmt(1))
	require.Equal(t, valFmt(1), sg)

	// make a new from st and check
	st2 := cachemulti.NewStore(st)
	sg2, _ := st2.Get(keyFmt(1))
	require.Equal(t, valFmt(1), sg2)

	// update the value on st2, check it only effects st2
	_ = st2.Set(keyFmt(1), valFmt(3))
	mg, _ = mem.Get(keyFmt(1))
	require.Equal(t, []byte(nil), mg)
	sg, _ = st.Get(keyFmt(1))
	sg2, _ = st2.Get(keyFmt(1))
	require.Equal(t, valFmt(1), sg)
	require.Equal(t, valFmt(3), sg2)

	// st2 writes to its parent, st. doesnt effect mem
	st2.Write()
	mg, _ = mem.Get(keyFmt(1))
	sg, _ = st.Get(keyFmt(1))
	require.Equal(t, []byte(nil), mg)
	require.Equal(t, valFmt(3), sg)

	// updates mem
	st.Write()
	mg, _ = mem.Get(keyFmt(1))
	require.Equal(t, valFmt(3), mg)
}

func TestCacheKVIteratorBounds(t *testing.T) {
	st := newCacheKVStore()

	// set some items
	nItems := 5
	for i := 0; i < nItems; i++ {
		_ = st.Set(keyFmt(i), valFmt(i))
	}

	// iterate over all of them
	itr, _ := st.Iterator(nil, nil)
	var i = 0
	for ; itr.Valid(); itr.Next() {
		k, v := itr.Key(), itr.Value()
		require.Equal(t, keyFmt(i), k)
		require.Equal(t, valFmt(i), v)
		i++
	}
	require.Equal(t, nItems, i)

	// iterate over none
	itr, _ = st.Iterator(bz("money"), nil)
	i = 0
	for ; itr.Valid(); itr.Next() {
		i++
	}
	require.Equal(t, 0, i)

	// iterate over lower
	itr, _ = st.Iterator(keyFmt(0), keyFmt(3))
	i = 0
	for ; itr.Valid(); itr.Next() {
		k, v := itr.Key(), itr.Value()
		require.Equal(t, keyFmt(i), k)
		require.Equal(t, valFmt(i), v)
		i++
	}
	require.Equal(t, 3, i)

	// iterate over upper
	itr, _ = st.Iterator(keyFmt(2), keyFmt(4))
	i = 2
	for ; itr.Valid(); itr.Next() {
		k, v := itr.Key(), itr.Value()
		require.Equal(t, keyFmt(i), k)
		require.Equal(t, valFmt(i), v)
		i++
	}
	require.Equal(t, 4, i)
}

func TestCacheKVMergeIteratorBasics(t *testing.T) {
	st := newCacheKVStore()

	// set and delete an item in the cache, iterator should be empty
	k, v := keyFmt(0), valFmt(0)
	_ = st.Set(k, v)
	_ = st.Delete(k)
	assertIterateDomain(t, st, 0)

	// now set it and assert its there
	_ = st.Set(k, v)
	assertIterateDomain(t, st, 1)

	// write it and assert its there
	st.Write()
	assertIterateDomain(t, st, 1)

	// remove it in cache and assert its not
	_ = st.Delete(k)
	assertIterateDomain(t, st, 0)

	// write the delete and assert its not there
	st.Write()
	assertIterateDomain(t, st, 0)

	// add two keys and assert theyre there
	k1, v1 := keyFmt(1), valFmt(1)
	_ = st.Set(k, v)
	_ = st.Set(k1, v1)
	assertIterateDomain(t, st, 2)

	// write it and assert theyre there
	st.Write()
	assertIterateDomain(t, st, 2)

	// remove one in cache and assert its not
	_ = st.Delete(k1)
	assertIterateDomain(t, st, 1)

	// write the delete and assert its not there
	st.Write()
	assertIterateDomain(t, st, 1)

	// delete the other key in cache and asserts its empty
	_ = st.Delete(k)
	assertIterateDomain(t, st, 0)
}

func TestCacheKVMergeIteratorDeleteLast(t *testing.T) {
	st := newCacheKVStore()

	// set some items and write them
	nItems := 5
	for i := 0; i < nItems; i++ {
		_ = st.Set(keyFmt(i), valFmt(i))
	}
	st.Write()

	// set some more items and leave dirty
	for i := nItems; i < nItems*2; i++ {
		_ = st.Set(keyFmt(i), valFmt(i))
	}

	// iterate over all of them
	assertIterateDomain(t, st, nItems*2)

	// delete them all
	for i := 0; i < nItems*2; i++ {
		last := nItems*2 - 1 - i
		_ = st.Delete(keyFmt(last))
		assertIterateDomain(t, st, last)
	}
}

func TestCacheKVMergeIteratorDeletes(t *testing.T) {
	st := newCacheKVStore()
	truth := dbm.NewMemDB()

	// set some items and write them
	nItems := 10
	for i := 0; i < nItems; i++ {
		doOp(st, truth, opSet, i)
	}
	st.Write()

	// delete every other item, starting from 0
	for i := 0; i < nItems; i += 2 {
		doOp(st, truth, opDel, i)
		assertIterateDomainCompare(t, st, truth)
	}

	// reset
	st = newCacheKVStore()
	truth = dbm.NewMemDB()

	// set some items and write them
	for i := 0; i < nItems; i++ {
		doOp(st, truth, opSet, i)
	}
	st.Write()

	// delete every other item, starting from 1
	for i := 1; i < nItems; i += 2 {
		doOp(st, truth, opDel, i)
		assertIterateDomainCompare(t, st, truth)
	}
}

func TestCacheKVMergeIteratorChunks(t *testing.T) {
	st := newCacheKVStore()

	// Use the truth to check values on the merge iterator
	truth := dbm.NewMemDB()

	// sets to the parent
	setRange(st, truth, 0, 20)
	setRange(st, truth, 40, 60)
	st.Write()

	// sets to the cache
	setRange(st, truth, 20, 40)
	setRange(st, truth, 60, 80)
	assertIterateDomainCheck(t, st, truth, []keyRange{{0, 80}})

	// remove some parents and some cache
	deleteRange(st, truth, 15, 25)
	assertIterateDomainCheck(t, st, truth, []keyRange{{0, 15}, {25, 80}})

	// remove some parents and some cache
	deleteRange(st, truth, 35, 45)
	assertIterateDomainCheck(t, st, truth, []keyRange{{0, 15}, {25, 35}, {45, 80}})

	// write, add more to the cache, and delete some cache
	st.Write()
	setRange(st, truth, 38, 42)
	deleteRange(st, truth, 40, 43)
	assertIterateDomainCheck(t, st, truth, []keyRange{{0, 15}, {25, 35}, {38, 40}, {45, 80}})
}

func TestCacheKVMergeIteratorRandom(t *testing.T) {
	st := newCacheKVStore()
	truth := dbm.NewMemDB()

	start, end := 25, 975
	max := 1000
	setRange(st, truth, start, end)

	// do an op, test the iterator
	for i := 0; i < 2000; i++ {
		doRandomOp(st, truth, max)
		assertIterateDomainCompare(t, st, truth)
	}
}

//-------------------------------------------------------------------------------------------
// do some random ops

const (
	opSet      = 0
	opSetRange = 1
	opDel      = 2
	opDelRange = 3
	opWrite    = 4

	totalOps = 5 // number of possible operations
)

func randInt(n int) int {
	return rand2.Int() % n
}

// useful for replaying a error case if we find one
func doOp(st types.CacheKVStore, truth dbm.DB, op int, args ...int) {
	switch op {
	case opSet:
		k := args[0]
		_ = st.Set(keyFmt(k), valFmt(k))
		_ = truth.Set(keyFmt(k), valFmt(k))
	case opSetRange:
		start := args[0]
		end := args[1]
		setRange(st, truth, start, end)
	case opDel:
		k := args[0]
		_ = st.Delete(keyFmt(k))
		_ = truth.Delete(keyFmt(k))
	case opDelRange:
		start := args[0]
		end := args[1]
		deleteRange(st, truth, start, end)
	case opWrite:
		st.Write()
	}
}

func doRandomOp(st types.CacheKVStore, truth dbm.DB, maxKey int) {
	r := randInt(totalOps)
	switch r {
	case opSet:
		k := randInt(maxKey)
		_ = st.Set(keyFmt(k), valFmt(k))
		_ = truth.Set(keyFmt(k), valFmt(k))
	case opSetRange:
		start := randInt(maxKey - 2)
		end := randInt(maxKey-start) + start
		setRange(st, truth, start, end)
	case opDel:
		k := randInt(maxKey)
		_ = st.Delete(keyFmt(k))
		_ = truth.Delete(keyFmt(k))
	case opDelRange:
		start := randInt(maxKey - 2)
		end := randInt(maxKey-start) + start
		deleteRange(st, truth, start, end)
	case opWrite:
		st.Write()
	}
}

//-------------------------------------------------------------------------------------------

// iterate over whole domain
func assertIterateDomain(t *testing.T, st types.KVStore, expectedN int) {
	itr, _ := st.Iterator(nil, nil)
	var i = 0
	for ; itr.Valid(); itr.Next() {
		k, v := itr.Key(), itr.Value()
		require.Equal(t, keyFmt(i), k)
		require.Equal(t, valFmt(i), v)
		i++
	}
	require.Equal(t, expectedN, i)
}

func assertIterateDomainCheck(t *testing.T, st types.KVStore, mem dbm.DB, r []keyRange) {
	// iterate over each and check they match the other
	itr, _ := st.Iterator(nil, nil)
	itr2, _ := mem.Iterator(nil, nil) // ground truth

	krc := newKeyRangeCounter(r)
	i := 0

	for ; krc.valid(); krc.next() {
		require.True(t, itr.Valid())
		require.True(t, itr2.Valid())

		// check the key/val matches the ground truth
		k, v := itr.Key(), itr.Value()
		k2, v2 := itr2.Key(), itr2.Value()
		require.Equal(t, k, k2)
		require.Equal(t, v, v2)

		// check they match the counter
		require.Equal(t, k, keyFmt(krc.key()))

		itr.Next()
		itr2.Next()
		i++
	}

	require.False(t, itr.Valid())
	require.False(t, itr2.Valid())
}

func assertIterateDomainCompare(t *testing.T, st types.KVStore, mem dbm.DB) {
	// iterate over each and check they match the other
	itr, _ := st.Iterator(nil, nil)
	itr2, _ := mem.Iterator(nil, nil) // ground truth
	checkIterators(t, itr, itr2)
	checkIterators(t, itr2, itr)
}

func checkIterators(t *testing.T, itr, itr2 types.Iterator) {
	for ; itr.Valid(); itr.Next() {
		require.True(t, itr2.Valid())
		k, v := itr.Key(), itr.Value()
		k2, v2 := itr2.Key(), itr2.Value()
		require.Equal(t, k, k2)
		require.Equal(t, v, v2)
		itr2.Next()
	}
	require.False(t, itr.Valid())
	require.False(t, itr2.Valid())
}

//--------------------------------------------------------

func setRange(st types.KVStore, mem dbm.DB, start, end int) {
	for i := start; i < end; i++ {
		_ = st.Set(keyFmt(i), valFmt(i))
		_ = mem.Set(keyFmt(i), valFmt(i))
	}
}

func deleteRange(st types.KVStore, mem dbm.DB, start, end int) {
	for i := start; i < end; i++ {
		_ = st.Delete(keyFmt(i))
		_ = mem.Delete(keyFmt(i))
	}
}

//--------------------------------------------------------

type keyRange struct {
	start int
	end   int
}

func (kr keyRange) len() int {
	return kr.end - kr.start
}

func newKeyRangeCounter(kr []keyRange) *keyRangeCounter {
	return &keyRangeCounter{keyRanges: kr}
}

// we can iterate over this and make sure our real iterators have all the right keys
type keyRangeCounter struct {
	rangeIdx  int
	idx       int
	keyRanges []keyRange
}

func (krc *keyRangeCounter) valid() bool {
	maxRangeIdx := len(krc.keyRanges) - 1
	maxRange := krc.keyRanges[maxRangeIdx]

	// if we're not in the max range, we're valid
	if krc.rangeIdx <= maxRangeIdx &&
		krc.idx < maxRange.len() {
		return true
	}

	return false
}

func (krc *keyRangeCounter) next() {
	thisKeyRange := krc.keyRanges[krc.rangeIdx]
	if krc.idx == thisKeyRange.len()-1 {
		krc.rangeIdx++
		krc.idx = 0
	} else {
		krc.idx++
	}
}

func (krc *keyRangeCounter) key() int {
	thisKeyRange := krc.keyRanges[krc.rangeIdx]
	return thisKeyRange.start + krc.idx
}

//--------------------------------------------------------

func bz(s string) []byte { return []byte(s) }

func BenchmarkCacheKVStoreGetNoKeyFound(b *testing.B) {
	st := newCacheKVStore()
	b.ResetTimer()
	// assumes b.N < 2**24
	for i := 0; i < b.N; i++ {
		_, _ = st.Get([]byte{byte((i & 0xFF0000) >> 16), byte((i & 0xFF00) >> 8), byte(i & 0xFF)})
	}
}

func BenchmarkCacheKVStoreGetKeyFound(b *testing.B) {
	st := newCacheKVStore()
	for i := 0; i < b.N; i++ {
		arr := []byte{byte((i & 0xFF0000) >> 16), byte((i & 0xFF00) >> 8), byte(i & 0xFF)}
		_ = st.Set(arr, arr)
	}
	b.ResetTimer()
	// assumes b.N < 2**24
	for i := 0; i < b.N; i++ {
		_, _ = st.Get([]byte{byte((i & 0xFF0000) >> 16), byte((i & 0xFF00) >> 8), byte(i & 0xFF)})
	}
}