package main

// Pure-Go RIPEMD-160 implementation (public domain-inspired minimal version)
// Implements the standard RIPEMD-160 hash.Hash interface subset used here.

type ripemd160State struct {
	h0, h1, h2, h3, h4 uint32
	x                  [16]uint32
	nx                 int
	len                uint64
}

type ripemd160Hash struct{ s ripemd160State }

func NewRIPEMD160() *ripemd160Hash {
	var h ripemd160Hash
	h.Reset()
	return &h
}

func (h *ripemd160Hash) Reset() {
	h.s.h0 = 0x67452301
	h.s.h1 = 0xefcdab89
	h.s.h2 = 0x98badcfe
	h.s.h3 = 0x10325476
	h.s.h4 = 0xc3d2e1f0
	h.s.nx = 0
	h.s.len = 0
}

func (h *ripemd160Hash) Write(p []byte) (int, error) {
	n := len(p)
	h.s.len += uint64(n)
	if h.s.nx > 0 {
		for h.s.nx < 64 && len(p) > 0 {
			h.s.x[h.s.nx>>2] |= uint32(p[0]) << (8 * (h.s.nx & 3))
			h.s.nx++
			p = p[1:]
		}
		if h.s.nx == 64 {
			block(&h.s)
			h.s.x = [16]uint32{}
			h.s.nx = 0
		}
	}
	for len(p) >= 64 {
		for i := 0; i < 16; i++ {
			h.s.x[i] = uint32(p[4*i]) | uint32(p[4*i+1])<<8 | uint32(p[4*i+2])<<16 | uint32(p[4*i+3])<<24
		}
		block(&h.s)
		p = p[64:]
	}
	for _, b := range p {
		h.s.x[h.s.nx>>2] |= uint32(b) << (8 * (h.s.nx & 3))
		h.s.nx++
	}
	return n, nil
}

func (h *ripemd160Hash) Sum(in []byte) []byte {
	// Compute digest on a copy to avoid mutating original state
	hh := *h
	// Append 0x80
	hh.Write([]byte{0x80})
	// Pad with zeros until 56 mod 64
	for (hh.s.nx % 64) != 56 {
		hh.Write([]byte{0x00})
	}
	// Length in bits (little-endian) from original length
	var lb [8]byte
	l := h.s.len * 8
	for i := 0; i < 8; i++ {
		lb[i] = byte(l >> (8 * uint(i)))
	}
	hh.Write(lb[:])
	// Output
	out := make([]byte, 20)
	putu32le(out[0:4], hh.s.h0)
	putu32le(out[4:8], hh.s.h1)
	putu32le(out[8:12], hh.s.h2)
	putu32le(out[12:16], hh.s.h3)
	putu32le(out[16:20], hh.s.h4)
	return append(in, out...)
}

func putu32le(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}

// RIPEMD-160 compression function
func block(s *ripemd160State) {
	// Constants and functions per spec
	f := func(x, y, z uint32) uint32 { return x ^ y ^ z }
	g := func(x, y, z uint32) uint32 { return (x & y) | (^x & z) }
	h := func(x, y, z uint32) uint32 { return (x | ^y) ^ z }
	i := func(x, y, z uint32) uint32 { return (x & z) | (y & ^z) }
	j := func(x, y, z uint32) uint32 { return x ^ (y | ^z) }

	rl := func(x uint32, n uint) uint32 { return (x<<n | x>>(32-n)) }

	var (
		a, b, c, d, e = s.h0, s.h1, s.h2, s.h3, s.h4
		A, B, C, D, E = s.h0, s.h1, s.h2, s.h3, s.h4
		X             = s.x
	)

	// Left line
	a = rl(a+f(b, c, d)+X[0], 11) + e
	c = rl(c, 10)
	e = rl(e+f(a, b, c)+X[1], 14) + d
	b = rl(b, 10)
	d = rl(d+f(e, a, b)+X[2], 15) + c
	a = rl(a, 10)
	c = rl(c+f(d, e, a)+X[3], 12) + b
	e = rl(e, 10)
	b = rl(b+f(c, d, e)+X[4], 5) + a
	d = rl(d, 10)
	a = rl(a+f(b, c, d)+X[5], 8) + e
	c = rl(c, 10)
	e = rl(e+f(a, b, c)+X[6], 7) + d
	b = rl(b, 10)
	d = rl(d+f(e, a, b)+X[7], 9) + c
	a = rl(a, 10)
	c = rl(c+f(d, e, a)+X[8], 11) + b
	e = rl(e, 10)
	b = rl(b+f(c, d, e)+X[9], 13) + a
	d = rl(d, 10)
	a = rl(a+f(b, c, d)+X[10], 14) + e
	c = rl(c, 10)
	e = rl(e+f(a, b, c)+X[11], 15) + d
	b = rl(b, 10)
	d = rl(d+f(e, a, b)+X[12], 6) + c
	a = rl(a, 10)
	c = rl(c+f(d, e, a)+X[13], 7) + b
	e = rl(e, 10)
	b = rl(b+f(c, d, e)+X[14], 9) + a
	d = rl(d, 10)
	a = rl(a+f(b, c, d)+X[15], 8) + e
	c = rl(c, 10)

	// Round 2
	const K1 = 0x5a827999
	a = rl(a+g(b, c, d)+X[7]+K1, 7) + e
	c = rl(c, 10)
	e = rl(e+g(a, b, c)+X[4]+K1, 6) + d
	b = rl(b, 10)
	d = rl(d+g(e, a, b)+X[13]+K1, 8) + c
	a = rl(a, 10)
	c = rl(c+g(d, e, a)+X[1]+K1, 13) + b
	e = rl(e, 10)
	b = rl(b+g(c, d, e)+X[10]+K1, 11) + a
	d = rl(d, 10)
	a = rl(a+g(b, c, d)+X[6]+K1, 9) + e
	c = rl(c, 10)
	e = rl(e+g(a, b, c)+X[15]+K1, 7) + d
	b = rl(b, 10)
	d = rl(d+g(e, a, b)+X[3]+K1, 15) + c
	a = rl(a, 10)
	c = rl(c+g(d, e, a)+X[12]+K1, 7) + b
	e = rl(e, 10)
	b = rl(b+g(c, d, e)+X[0]+K1, 12) + a
	d = rl(d, 10)
	a = rl(a+g(b, c, d)+X[9]+K1, 15) + e
	c = rl(c, 10)
	e = rl(e+g(a, b, c)+X[5]+K1, 9) + d
	b = rl(b, 10)
	d = rl(d+g(e, a, b)+X[2]+K1, 11) + c
	a = rl(a, 10)
	c = rl(c+g(d, e, a)+X[14]+K1, 7) + b
	e = rl(e, 10)
	b = rl(b+g(c, d, e)+X[11]+K1, 13) + a
	d = rl(d, 10)
	a = rl(a+g(b, c, d)+X[8]+K1, 12) + e
	c = rl(c, 10)

	// Round 3
	const K2 = 0x6ed9eba1
	a = rl(a+h(b, c, d)+X[3]+K2, 11) + e
	c = rl(c, 10)
	e = rl(e+h(a, b, c)+X[10]+K2, 13) + d
	b = rl(b, 10)
	d = rl(d+h(e, a, b)+X[14]+K2, 6) + c
	a = rl(a, 10)
	c = rl(c+h(d, e, a)+X[4]+K2, 7) + b
	e = rl(e, 10)
	b = rl(b+h(c, d, e)+X[9]+K2, 14) + a
	d = rl(d, 10)
	a = rl(a+h(b, c, d)+X[15]+K2, 9) + e
	c = rl(c, 10)
	e = rl(e+h(a, b, c)+X[8]+K2, 13) + d
	b = rl(b, 10)
	d = rl(d+h(e, a, b)+X[1]+K2, 15) + c
	a = rl(a, 10)
	c = rl(c+h(d, e, a)+X[2]+K2, 14) + b
	e = rl(e, 10)
	b = rl(b+h(c, d, e)+X[7]+K2, 8) + a
	d = rl(d, 10)
	a = rl(a+h(b, c, d)+X[0]+K2, 13) + e
	c = rl(c, 10)
	e = rl(e+h(a, b, c)+X[6]+K2, 6) + d
	b = rl(b, 10)
	d = rl(d+h(e, a, b)+X[13]+K2, 5) + c
	a = rl(a, 10)
	c = rl(c+h(d, e, a)+X[11]+K2, 12) + b
	e = rl(e, 10)
	b = rl(b+h(c, d, e)+X[5]+K2, 7) + a
	d = rl(d, 10)
	a = rl(a+h(b, c, d)+X[12]+K2, 5) + e
	c = rl(c, 10)

	// Right line
	const Kr0 = 0x50a28be6
	A = rl(A+j(B, C, D)+X[5]+Kr0, 8) + E
	C = rl(C, 10)
	E = rl(E+j(A, B, C)+X[14]+Kr0, 9) + D
	B = rl(B, 10)
	D = rl(D+j(E, A, B)+X[7]+Kr0, 9) + C
	A = rl(A, 10)
	C = rl(C+j(D, E, A)+X[0]+Kr0, 11) + B
	E = rl(E, 10)
	B = rl(B+j(C, D, E)+X[9]+Kr0, 13) + A
	D = rl(D, 10)
	A = rl(A+j(B, C, D)+X[2]+Kr0, 15) + E
	C = rl(C, 10)
	E = rl(E+j(A, B, C)+X[11]+Kr0, 15) + D
	B = rl(B, 10)
	D = rl(D+j(E, A, B)+X[4]+Kr0, 5) + C
	A = rl(A, 10)
	C = rl(C+j(D, E, A)+X[13]+Kr0, 7) + B
	E = rl(E, 10)
	B = rl(B+j(C, D, E)+X[6]+Kr0, 7) + A
	D = rl(D, 10)
	A = rl(A+j(B, C, D)+X[15]+Kr0, 8) + E
	C = rl(C, 10)
	E = rl(E+j(A, B, C)+X[8]+Kr0, 11) + D
	B = rl(B, 10)
	D = rl(D+j(E, A, B)+X[1]+Kr0, 14) + C
	A = rl(A, 10)
	C = rl(C+j(D, E, A)+X[10]+Kr0, 14) + B
	E = rl(E, 10)
	B = rl(B+j(C, D, E)+X[3]+Kr0, 12) + A
	D = rl(D, 10)
	A = rl(A+j(B, C, D)+X[12]+Kr0, 6) + E
	C = rl(C, 10)

	const Kr1 = 0x5c4dd124
	A = rl(A+i(B, C, D)+X[6]+Kr1, 9) + E
	C = rl(C, 10)
	E = rl(E+i(A, B, C)+X[11]+Kr1, 13) + D
	B = rl(B, 10)
	D = rl(D+i(E, A, B)+X[3]+Kr1, 15) + C
	A = rl(A, 10)
	C = rl(C+i(D, E, A)+X[7]+Kr1, 7) + B
	E = rl(E, 10)
	B = rl(B+i(C, D, E)+X[0]+Kr1, 12) + A
	D = rl(D, 10)
	A = rl(A+i(B, C, D)+X[13]+Kr1, 8) + E
	C = rl(C, 10)
	E = rl(E+i(A, B, C)+X[5]+Kr1, 9) + D
	B = rl(B, 10)
	D = rl(D+i(E, A, B)+X[10]+Kr1, 11) + C
	A = rl(A, 10)
	C = rl(C+i(D, E, A)+X[14]+Kr1, 7) + B
	E = rl(E, 10)
	B = rl(B+i(C, D, E)+X[15]+Kr1, 7) + A
	D = rl(D, 10)
	A = rl(A+i(B, C, D)+X[8]+Kr1, 12) + E
	C = rl(C, 10)
	E = rl(E+i(A, B, C)+X[12]+Kr1, 7) + D
	B = rl(B, 10)
	D = rl(D+i(E, A, B)+X[4]+Kr1, 6) + C
	A = rl(A, 10)
	C = rl(C+i(D, E, A)+X[9]+Kr1, 15) + B
	E = rl(E, 10)
	B = rl(B+i(C, D, E)+X[1]+Kr1, 13) + A
	D = rl(D, 10)
	A = rl(A+i(B, C, D)+X[2]+Kr1, 11) + E
	C = rl(C, 10)

	const Kr2 = 0x6d703ef3
	A = rl(A+h(B, C, D)+X[15]+Kr2, 9) + E
	C = rl(C, 10)
	E = rl(E+h(A, B, C)+X[5]+Kr2, 7) + D
	B = rl(B, 10)
	D = rl(D+h(E, A, B)+X[1]+Kr2, 15) + C
	A = rl(A, 10)
	C = rl(C+h(D, E, A)+X[3]+Kr2, 11) + B
	E = rl(E, 10)
	B = rl(B+h(C, D, E)+X[7]+Kr2, 8) + A
	D = rl(D, 10)
	A = rl(A+h(B, C, D)+X[14]+Kr2, 6) + E
	C = rl(C, 10)
	E = rl(E+h(A, B, C)+X[6]+Kr2, 6) + D
	B = rl(B, 10)
	D = rl(D+h(E, A, B)+X[9]+Kr2, 14) + C
	A = rl(A, 10)
	C = rl(C+h(D, E, A)+X[11]+Kr2, 12) + B
	E = rl(E, 10)
	B = rl(B+h(C, D, E)+X[8]+Kr2, 13) + A
	D = rl(D, 10)
	A = rl(A+h(B, C, D)+X[12]+Kr2, 5) + E
	C = rl(C, 10)
	E = rl(E+h(A, B, C)+X[2]+Kr2, 14) + D
	B = rl(B, 10)
	D = rl(D+h(E, A, B)+X[10]+Kr2, 13) + C
	A = rl(A, 10)
	C = rl(C+h(D, E, A)+X[0]+Kr2, 13) + B
	E = rl(E, 10)
	B = rl(B+h(C, D, E)+X[4]+Kr2, 7) + A
	D = rl(D, 10)
	A = rl(A+h(B, C, D)+X[13]+Kr2, 5) + E
	C = rl(C, 10)

	const Kr3 = 0x7a6d76e9
	A = rl(A+g(B, C, D)+X[8]+Kr3, 15) + E
	C = rl(C, 10)
	E = rl(E+g(A, B, C)+X[6]+Kr3, 5) + D
	B = rl(B, 10)
	D = rl(D+g(E, A, B)+X[4]+Kr3, 8) + C
	A = rl(A, 10)
	C = rl(C+g(D, E, A)+X[1]+Kr3, 11) + B
	E = rl(E, 10)
	B = rl(B+g(C, D, E)+X[3]+Kr3, 14) + A
	D = rl(D, 10)
	A = rl(A+g(B, C, D)+X[11]+Kr3, 14) + E
	C = rl(C, 10)
	E = rl(E+g(A, B, C)+X[15]+Kr3, 6) + D
	B = rl(B, 10)
	D = rl(D+g(E, A, B)+X[0]+Kr3, 14) + C
	A = rl(A, 10)
	C = rl(C+g(D, E, A)+X[5]+Kr3, 6) + B
	E = rl(E, 10)
	B = rl(B+g(C, D, E)+X[12]+Kr3, 9) + A
	D = rl(D, 10)
	A = rl(A+g(B, C, D)+X[2]+Kr3, 12) + E
	C = rl(C, 10)
	E = rl(E+g(A, B, C)+X[13]+Kr3, 9) + D
	B = rl(B, 10)
	D = rl(D+g(E, A, B)+X[9]+Kr3, 12) + C
	A = rl(A, 10)
	C = rl(C+g(D, E, A)+X[7]+Kr3, 5) + B
	E = rl(E, 10)
	B = rl(B+g(C, D, E)+X[10]+Kr3, 15) + A
	D = rl(D, 10)
	A = rl(A+g(B, C, D)+X[14]+Kr3, 8) + E
	C = rl(C, 10)

	const Kr4 = 0x00000000
	A = rl(A+f(B, C, D)+X[12]+Kr4, 8) + E
	C = rl(C, 10)
	E = rl(E+f(A, B, C)+X[15]+Kr4, 5) + D
	B = rl(B, 10)
	D = rl(D+f(E, A, B)+X[10]+Kr4, 12) + C
	A = rl(A, 10)
	C = rl(C+f(D, E, A)+X[4]+Kr4, 9) + B
	E = rl(E, 10)
	B = rl(B+f(C, D, E)+X[1]+Kr4, 12) + A
	D = rl(D, 10)
	A = rl(A+f(B, C, D)+X[5]+Kr4, 5) + E
	C = rl(C, 10)
	E = rl(E+f(A, B, C)+X[8]+Kr4, 14) + D
	B = rl(B, 10)
	D = rl(D+f(E, A, B)+X[7]+Kr4, 6) + C
	A = rl(A, 10)
	C = rl(C+f(D, E, A)+X[6]+Kr4, 8) + B
	E = rl(E, 10)
	B = rl(B+f(C, D, E)+X[2]+Kr4, 13) + A
	D = rl(D, 10)
	A = rl(A+f(B, C, D)+X[13]+Kr4, 6) + E
	C = rl(C, 10)
	E = rl(E+f(A, B, C)+X[14]+Kr4, 5) + D
	B = rl(B, 10)
	D = rl(D+f(E, A, B)+X[0]+Kr4, 15) + C
	A = rl(A, 10)
	C = rl(C+f(D, E, A)+X[3]+Kr4, 13) + B
	E = rl(E, 10)
	B = rl(B+f(C, D, E)+X[9]+Kr4, 11) + A
	D = rl(D, 10)
	A = rl(A+f(B, C, D)+X[11]+Kr4, 11) + E
	C = rl(C, 10)

	// Combine results
	t := s.h1 + c + D
	s.h1 = s.h2 + d + E
	s.h2 = s.h3 + e + A
	s.h3 = s.h4 + a + B
	s.h4 = s.h0 + b + C
	s.h0 = t

	// Clear X for next block
	s.x = [16]uint32{}
}
