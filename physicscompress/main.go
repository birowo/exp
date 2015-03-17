// Response to Data Compression Challenge
// http://gafferongames.com/2015/03/14/the-networked-physics-data-compression-challenge/
//
// The main insight that this compression uses is
// given some previous state P and current state S
// the ordering by some attribute is very similar
//   order(P, attr) ~= order(S, attr)
//
// so we use the previous state to determine the order
// and delta encode the current attributes in that order
//
// Note that the ordering must give the same result on both computers
// but, it doesn't have to guarantee perfect ordering (e.g. see improveApprox)
//
// Anyways, the all the interesting stats
//
// #2831 337.941kbps ±152.762kbps
//
// MIN      4.320 kbps
// P05     13.920 kbps
// P10    120.000 kbps
// P25    243.840 kbps
// P50    349.440 kbps
// P75    444.000 kbps
// P90    525.600 kbps
// P95    578.880 kbps
// MAX    661.920 kbps
//
// TOTAL   15945.160 kb
//   AVG       5.632 kb per frame
//   AVG       6.251 bits per cube
//
// TIMING:
//                   MIN        10%        25%        50%        75%        90%        MAX
//    improve  609.236µs  734.746µs  795.491µs   854.45µs  916.535µs  976.386µs 2.951939ms
//     encode  211.714µs  356.876µs  443.974µs  508.292µs  564.124µs  611.023µs 1.047404ms
//     decode   39.305µs   85.311µs  144.716µs  192.508µs  234.046µs  267.992µs  460.054µs
package main

import (
	"bufio"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"runtime"
	"sort"

	"github.com/egonelbre/exp/qpc"

	"github.com/montanaflynn/stats"
)

const dontpack = false
const dontsort = false
const debugsnap = true

func check(err error) {
	if err != nil {
		panic(err)
	}
}

type Delta struct {
	Largest     int32
	A, B, C     int32
	X, Y, Z     int32
	Interacting int32
}

type Deltas []Delta

var previous_nochange []bool

func Encode(order *Ordering, baseline, current Deltas) (snapshot []byte) {
	wr := NewWriter()

	nochange := make([]bool, len(baseline))
	for i := range baseline {
		nochange[i] = baseline[i] == current[i]
	}
	previous_nochange = nochange
	wr.WriteBools(nochange)
	wr.WriteDelta(nochange, order.Largest, current, getLargest)
	wr.WriteDelta(nochange, order.Interacting, current, getInteracting)
	wr.WriteIndexed(nochange, order.ABC, baseline, current)
	wr.WriteIndexed(nochange, order.XYZ, baseline, current)

	wr.Close()
	return wr.Bytes()
}

// ignores reading errors
func Decode(order *Ordering, baseline, current Deltas, snapshot []byte) {
	rd := NewReader(snapshot)

	nochange := make([]bool, len(baseline))
	rd.ReadBools(nochange)

	copy(current, baseline)

	rd.ReadDelta(nochange, order.Largest, current, setLargest)
	rd.ReadDelta(nochange, order.Interacting, current, setInteracting)
	rd.ReadIndexed(nochange, order.ABC, baseline, current)
	rd.ReadIndexed(nochange, order.XYZ, baseline, current)
}

func ReadDelta(r io.Reader, current Deltas) error {
	for i := range current {
		if err := current[i].ReadFrom(r); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	verbose := flag.Bool("v", false, "verbose output")
	flag.Parse()

	file, err := os.Open("delta_data.bin")
	check(err)
	defer file.Close()

	buffer := bufio.NewReader(file)

	sizes := make([]float64, 0)
	speeds := make([]float64, 0)

	improve := qpc.NewHistory("improve")
	encode := qpc.NewHistory("encode")
	decode := qpc.NewHistory("decode")

	const N = 901
	baseline := make(Deltas, N)
	order := NewOrdering(baseline)

	var history [8]Deltas
	for i := range history {
		history[i] = make(Deltas, N)
	}

	frame := 0
	for i := 0; i < 6; i += 1 {
		ReadDelta(buffer, history[frame])
		frame += 1
	}

	mirror := make(Deltas, N)

	for {
		historic := history[(frame-7+len(history))%len(history)]
		baseline := history[(frame-6+len(history))%len(history)]
		current := history[frame%len(history)]

		err = ReadDelta(buffer, current)
		if err == io.EOF {
			break
		}
		check(err)
		frame += 1

		runtime.GC()

		//===
		improve.Start()
		order.Improve(historic, baseline)
		improve.Stop()

		encode.Start()
		snapshot := Encode(order, baseline, current)
		encode.Stop()

		if debugsnap && (frame == 1000 || frame == 1200 || frame == 1201 || frame == 1770) {
			size := float64(len(snapshot)*8) / 1000.0
			speed := size * 60
			fmt.Printf("\n<%d,%.3fkb,%.3fkbps>\n", frame, size, speed)
			ioutil.WriteFile(fmt.Sprintf("snapshot_%d.bin", frame), snapshot, 0755)
		}

		decode.Start()
		Decode(order, baseline, mirror, snapshot)
		decode.Stop()
		//===

		size := float64(len(snapshot)*8) / 1000.0
		sizes = append(sizes, size)

		speed := size * 60.0
		speeds = append(speeds, speed)

		if *verbose {
			if !current.Equals(mirror) {
				fmt.Print("! ")
			}
			fmt.Printf("%04d %8.3fkbps %10s %10s %10s\n", frame, speed, improve.Last(), encode.Last(), decode.Last())
		} else {
			if current.Equals(mirror) {
				fmt.Print(".")
			} else {
				fmt.Print("X")
			}
		}
	}

	fmt.Println()
	fmt.Printf("#%d %.3fkbps ±%.3fkbps\n", len(sizes), stats.Mean(speeds), stats.StdDevS(speeds))
	fmt.Println()

	fmt.Printf("MIN %10.3f kbps\n", stats.Min(speeds))
	for _, p := range []float64{5, 10, 25, 50, 75, 90, 95} {
		fmt.Printf("P%02.f %10.3f kbps\n", p, stats.Percentile(speeds, p))
	}
	fmt.Printf("MAX %10.3f kbps\n", stats.Max(speeds))

	fmt.Println()

	fmt.Printf("TOTAL  %10.3f kb\n", stats.Sum(sizes))
	fmt.Printf("  AVG  %10.3f kb per frame\n", stats.Mean(sizes))
	fmt.Printf("  AVG  %10.3f bits per cube\n", stats.Mean(sizes)*1000/float64(N))

	fmt.Println()
	fmt.Println("TIMING:")
	qpc.PrintSummary(improve, encode, decode)
}

// delta utilities
func (d *Delta) ReadFrom(r io.Reader) error {
	return read(r, &d.Largest, &d.A, &d.B, &d.C, &d.X, &d.Y, &d.Z, &d.Interacting)
}

func (d *Delta) WriteTo(w io.Writer) error {
	return write(w, &d.Largest, &d.A, &d.B, &d.C, &d.X, &d.Y, &d.Z, &d.Interacting)
}

// for validating content
func (a Deltas) Equals(b Deltas) bool {
	if len(a) != len(b) {
		return false
	}
	for i, ax := range a {
		if ax != b[i] {
			return false
		}
	}
	return true
}

// getters / setters
func getLargest(a *Delta) int32     { return a.Largest }
func getInteracting(a *Delta) int32 { return a.Interacting }

func setLargest(a *Delta, v int32)     { a.Largest = v }
func setInteracting(a *Delta, v int32) { a.Interacting = v }

// Order management

type Ordering struct {
	Largest     []int
	Interacting []int
	ABC         []IndexValue
	XYZ         []IndexValue
}

func NewOrdering(deltas Deltas) *Ordering {
	n := len(deltas)
	order := &Ordering{
		Largest:     make([]int, n),
		Interacting: make([]int, n),

		ABC: make([]IndexValue, n*3),
		XYZ: make([]IndexValue, n*3),
	}

	for i := range order.Largest {
		order.Largest[i] = i
		order.Interacting[i] = i
	}

	for i := 0; i < n*3; i += 1 {
		order.ABC[i].Index = i % n
		order.ABC[i].Value = byte(i / n)

		order.XYZ[i].Index = i % n
		order.XYZ[i].Value = byte(i/n + 3)
	}

	return order
}

var improve = improveSort

func improveSort(order []int, deltas Deltas, get Getter) {
	sort.Sort(&byGetter{order, deltas, get})
}

func improveApprox(order []int, deltas Deltas, get Getter) {
	const max_steps = 10
	for k := 0; k < max_steps; k += 1 {
		prev := int32(-(1 << 31))
		swapped := false
		for i, idx := range order {
			curr := get(&deltas[idx])
			if prev > curr {
				order[i-1], order[i] = order[i], order[i-1]
				swapped = true
			} else {
				prev = curr
			}
		}
		if !swapped {
			return
		}
	}
}

func (order *Ordering) Improve(historic, baseline Deltas) {
	if !dontsort {
		sort.Sort(&byGetter{order.Largest, baseline, getLargest})
		sort.Sort(&byGetter{order.Interacting, baseline, getInteracting})
		sort.Sort(&byDelta{order.ABC, historic, baseline})
		sort.Sort(&byDelta{order.XYZ, historic, baseline})
	}
}

// reading input/output
func read(r io.Reader, vs ...interface{}) error {
	for _, v := range vs {
		err := binary.Read(r, binary.LittleEndian, v)
		if err != nil {
			return err
		}
	}
	return nil
}

func write(w io.Writer, vs ...interface{}) error {
	for _, v := range vs {
		err := binary.Write(w, binary.LittleEndian, v)
		if err != nil {
			return err
		}
	}
	return nil
}
