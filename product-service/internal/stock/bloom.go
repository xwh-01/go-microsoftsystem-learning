package stock

import (
	"encoding/binary"
	"hash/fnv"
)

type BloomFilter struct {
	bits []byte
	m    uint64
	k    uint64
}

func NewBloomFilter(bitSize uint64, hashFunctions uint64) *BloomFilter {
	if bitSize == 0 {
		bitSize = 1024
	}
	if hashFunctions == 0 {
		hashFunctions = 3
	}
	return &BloomFilter{
		bits: make([]byte, (bitSize+7)/8),
		m:    bitSize,
		k:    hashFunctions,
	}
}

func (f *BloomFilter) AddInt32(v int32) {
	for i := uint64(0); i < f.k; i++ {
		idx := f.hashInt32(v, i) % f.m
		f.bits[idx/8] |= 1 << (idx % 8)
	}
}

func (f *BloomFilter) MightContainInt32(v int32) bool {
	for i := uint64(0); i < f.k; i++ {
		idx := f.hashInt32(v, i) % f.m
		if f.bits[idx/8]&(1<<(idx%8)) == 0 {
			return false
		}
	}
	return true
}

func (f *BloomFilter) hashInt32(v int32, seed uint64) uint64 {
	h := fnv.New64a()
	var b [12]byte
	binary.LittleEndian.PutUint64(b[0:8], seed)
	binary.LittleEndian.PutUint32(b[8:12], uint32(v))
	_, _ = h.Write(b[:])
	return h.Sum64()
}
