package physics

import (
	"flag"
	"fmt"

	"github.com/egonelbre/exp/bit"
	"github.com/egonelbre/exp/coder/arith"
)

var flagFlate = flag.Bool("flate", true, "use flate compression")

func encode32(v int32) uint64 { return uint64(bit.ZEncode(int64(v))) }
func decode32(v uint64) int32 { return int32(bit.ZDecode(v)) }

func write(w *bit.Writer, v int32, bits uint) { w.WriteBits(uint64(v), bits) }
func read(r *bit.Reader, bits uint) int32     { return int32(r.ReadBits(bits)) }

func write32(w *bit.Writer, v int32, bits uint) { w.WriteBits(bit.ZEncode(int64(v)), bits) }
func read32(r *bit.Reader, bits uint) int32     { return int32(bit.ZDecode(r.ReadBits(bits))) }

func maxbits(vs ...int32) (r uint) {
	r = 0
	for _, x := range vs {
		if x != 0 {
			bits := bit.ScanRight(bit.ZEncode(int64(x))) + 1
			if r < bits {
				r = bits
			}
		}
	}
	return
}

func deltabits(base, cube *Cube) uint {
	return maxbits(
		cube.Interacting^base.Interacting,
		cube.Largest^base.Largest,
		cube.A^base.A,
		cube.B^base.B,
		cube.C^base.C,
		cube.X^base.X,
		cube.Y^base.Y,
		cube.Z^base.Z,
	)
}

// encodes zeros well
func mZeros() arith.Model {
	return &arith.Shift2{P0: 0x34a, I0: 0x1, P1: 0xd0, I1: 0x5}
}

func mBits() arith.Model {
	return &arith.Shift{P: arith.MaxP * 2 / 3, I: 0x5}
}

func mRandom() arith.Model {
	return &arith.Shift2{P0: 0x500, I0: 0x1, P1: 0x150, I1: 0x5}
}

var totalbits = 0

func (s *State) Encode() []byte {
	enc := arith.NewEncoder()

	historic := s.Historic().Cubes
	_ = historic
	baseline := s.Baseline().Cubes
	current := s.Current().Cubes

	mzeros := mZeros()
	items := []int{}
	for i := range current {
		cube, base := &current[i], &baseline[i]
		if *cube == *base {
			mzeros.Encode(enc, 0)
		} else {
			mzeros.Encode(enc, 1)
			items = append(items, i)
		}
	}

	for _, i := range items {
		cube := &current[i]
		mzeros.Encode(enc, uint(cube.Interacting^1))
	}

	for _, i := range items {
		cube, base := &current[i], &baseline[i]
		v := uint(cube.Largest ^ base.Largest)
		mzeros.Encode(enc, v&1)
		mzeros.Encode(enc, v>>1)
	}

	items6 := index6(items, len(baseline))
	cur6 := Delta6(historic, baseline)
	ext6 := Extra6(historic, baseline, current)

	SortByZ(items6, cur6)
	for _, i := range items6 {
		ext := uint64(bit.ZEncode(int64(ext6(i))))

		if ext != 0 {
			totalext += int(bit.ScanRight(ext) + 1)
		}
		if cur != 0 {
			totalcur += int(bit.ScanRight(cur) + 1)
		}
	}

	/*
		items6 := index6(items, len(baseline))
		old6 := Delta6(historic, baseline)
		SortByZ(items6, old6)

		mrand := mZeros()
		cur6 := Delta6(historic, current)

		for _, i := range items6 {
			v := uint64(bit.ZEncode(int64(cur6(i))))
			if v == 0 {
				mzeros.Encode(enc, 0)
				continue
			}

			nbits := int(bit.ScanRight(v) + 1)
			if nbits < 5 {
				nbits = 5
			}

			if nbits&1 == 1 {
				nbits += 1
			}
			for i := 5; i < nbits; i += 2 {
				mzeros.Encode(enc, 1)
			}
			mzeros.Encode(enc, 0)

			rbits := uint(bit.Reverse(v, uint(nbits)))
			for i := 0; i < nbits; i += 1 {
				mrand.Encode(enc, rbits&1)
				rbits >>= 1
			}
		}
	*/
	enc.Close()
	return enc.Bytes()
}

func (s *State) Decode(snapshot []byte) {
	dec := arith.NewDecoder(snapshot)

	s.Current().Assign(s.Baseline())
	baseline := s.Baseline().Cubes
	current := s.Current().Cubes

	mzeros := mZeros()
	items := []int{}
	for i := range current {
		if mzeros.Decode(dec) == 1 {
			items = append(items, i)
		}
	}

	for _, i := range items {
		cube := &current[i]
		cube.Interacting = int32(mzeros.Decode(dec) ^ 1)
	}

	for _, i := range items {
		cube, base := &current[i], &baseline[i]
		v0, v1 := mzeros.Decode(dec), mzeros.Decode(dec)
		cube.Largest = base.Largest ^ int32(v1<<1|v0)
	}

	return
}

func index6(index []int, N int) []int {
	r := make([]int, 0, len(index)*6)
	for _, v := range index {
		r = append(r, v, v+N, v+N*2, v+N*3, v+N*4, v+N*5)
	}
	return r
}

func Value6(base []Cube) func(i int) int32 {
	N := len(base)
	return func(i int) int32 {
		k := i % N
		switch i / N {
		case 0:
			return base[k].A
		case 1:
			return base[k].B
		case 2:
			return base[k].C
		case 3:
			return base[k].X
		case 4:
			return base[k].Y
		case 5:
			return base[k].Z
		default:
			panic("invalid")
		}
	}
}

func Delta6(hist, base []Cube) func(i int) int32 {
	N := len(base)
	return func(i int) int32 {
		k := i % N
		switch i / N {
		case 0:
			return hist[k].B - base[k].A
		case 1:
			return hist[k].B - base[k].B
		case 2:
			return hist[k].C - base[k].C
		case 3:
			return hist[k].X - base[k].X
		case 4:
			return hist[k].Y - base[k].Y
		case 5:
			return hist[k].Z - base[k].Z
		default:
			panic("invalid")
		}
	}
}

func Extra6(hist, base, cur []Cube) func(i int) int32 {
	N := len(base)
	return func(i int) int32 {
		k := i % N
		switch i / N {
		case 0:
			return cur[k].A - base[k].A + (hist[k].B - base[k].A)
		case 1:
			return cur[k].B - base[k].B + (hist[k].B - base[k].B)
		case 2:
			return cur[k].C - base[k].C + (hist[k].C - base[k].C)
		case 3:
			return cur[k].X - base[k].X + (hist[k].X - base[k].X)
		case 4:
			return cur[k].Y - base[k].Y + (hist[k].Y - base[k].Y)
		case 5:
			return cur[k].Z - base[k].Z + (hist[k].Z - base[k].Z)
		default:
			panic("invalid")
		}
	}
}

func SameBools(a, b []bool) {
	if len(a) != len(b) {
		panic("different length")
	}

	as := BoolsStr(a)
	bs := BoolsStr(b)
	if as != bs {
		fmt.Println("---")
		fmt.Println(as)
		fmt.Println(bs)
		fmt.Println("---")
	}
}

func BoolsStr(a []bool) string {
	r := make([]byte, 0, len(a))
	for _, v := range a {
		if v {
			r = append(r, '.')
		} else {
			r = append(r, 'X')
		}
	}
	return string(r)
}
