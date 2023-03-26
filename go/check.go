package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"sync"

	"github.com/alexflint/go-arg"
	"github.com/dgryski/go-metro"
	"github.com/zeebo/xxh3"
	"github.com/zhangyunhao116/wyhash"
	"github.com/zhenjl/cityhash"
	"golang.org/x/exp/mmap"
)

type Task struct {
	absolutePath string
	hash         []byte
}

type Input []string

type Args struct {
	Colon      bool
	Cpuprofile bool
	Sha256     bool
	Xxh3       bool
	City       bool
	Metro      bool
	Strict     bool
}

var args Args

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func uint64ToBytes(num uint64) []byte {
	bt := make([]byte, 8)

	bt[0] = byte(num)
	bt[1] = byte(num >> 8)
	bt[2] = byte(num >> 16)
	bt[3] = byte(num >> 24)
	bt[4] = byte(num >> 32)
	bt[5] = byte(num >> 40)
	bt[6] = byte(num >> 48)
	bt[7] = byte(num >> 56)
	return bt
}

func fmtHex(num []byte) string {

	var sb strings.Builder
	for i, b := range num {
		sb.WriteString(fmt.Sprintf("%02X", b))

		if i < len(num)-1 {
			sb.WriteString(":")
		}
	}

	return sb.String()
}

func getHashForFile(path string, args Args) ([]byte, error) {

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := int(fi.Size())

	data := make([]byte, size)

	if size > 24*1024 {
		f2, err := mmap.Open(path)
		defer f2.Close()

		if err != nil {
			return nil, err
		}

		_, err = f2.ReadAt(data, 0)
		if err != nil {
			return nil, err
		}
	} else {
		_, err = f.ReadAt(data, 0)
		if err != nil {
			return nil, err
		}
	}

	if args.Sha256 {
		h := sha256.New()
		h.Write(data)
		return h.Sum(nil), nil
	} else if args.Xxh3 {
		h := xxh3.HashSeed(data, 1)
		return uint64ToBytes(h), nil
	} else if args.City {
		h := cityhash.CityHash64WithSeed(data, uint32(len(data)), 1)
		return uint64ToBytes(h), nil
	} else if args.Metro {
		h := metro.Hash64(data, 1)
		return uint64ToBytes(h), nil
	} else {
		h := wyhash.NewDefault()
		h.Write(data)
		return uint64ToBytes(h.Sum64()), nil
	}
}

func (i *Input) Len() int {
	return len(*i)
}

func (i *Input) Swap(l, r int) {
	(*i)[l], (*i)[r] = (*i)[r], (*i)[l]
}

func (i *Input) Less(l, r int) bool {
	return (*i)[l] < (*i)[r]
}

func main() {

	arg.MustParse(&args)

	if args.Cpuprofile {
		f, err := os.Create(".profile")
		if err != nil {
			panic(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	var tasks []Task

	data, err := io.ReadAll(os.Stdin)

	if err != nil {
		panic(fmt.Errorf("Error reading stdin"))
	}

	lines := Input(strings.Split(string(data), "\n"))
	sort.Sort(&lines)

	for _, l := range lines {
		if l != "" {

			if args.Sha256 {
				tasks = append(tasks, Task{l, make([]byte, 32)})
			} else {
				tasks = append(tasks, Task{l, make([]byte, 8)})
			}

		}
	}

	if len(tasks) == 0 {
		fmt.Printf("%X\n", 0)
		return
	}

	partSize := len(tasks) / runtime.NumCPU()

	var wg sync.WaitGroup
	for start, end := 0, 0; end < len(tasks); {
		end = min(start+partSize, len(tasks))

		wg.Add(1)

		go func(tasks []Task) {
			defer wg.Done()

			for idx, _ := range tasks {
				ret, err := getHashForFile(tasks[idx].absolutePath, args)
				if err != nil {
					os.Stderr.WriteString(fmt.Sprintf("%s: %s\n", tasks[idx].absolutePath, err))

					if args.Strict {
						panic(err)
					} else {
						continue
					}
				}
				tasks[idx].hash = ret
			}

		}(tasks[start:end])

		start = end
	}

	wg.Wait()

	var sb strings.Builder
	for _, task := range tasks {
		if args.Colon {
			sb.WriteString(fmtHex(task.hash))
		} else {
			sb.WriteString(hex.EncodeToString(task.hash))
		}

		sb.WriteString(" " + task.absolutePath)

		sb.WriteString("\n")
	}

	fmt.Print(sb.String())
}
