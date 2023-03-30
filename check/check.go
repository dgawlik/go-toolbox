package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"reflect"
	"regexp"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"sync"

	"github.com/alexflint/go-arg"
	"github.com/zhangyunhao116/wyhash"
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
	Strict     bool
	Check      string
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

func fmtHex(num []byte, args Args) string {

	var sb strings.Builder

	for i, b := range num {

		sb.WriteString(fmt.Sprintf("%02X", b))

		if i < len(num)-1 && args.Colon {
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

	_, err = f.ReadAt(data, 0)

	if err != nil {
		return nil, err
	}

	if args.Sha256 {

		h := sha256.New()
		h.Write(data)
		return h.Sum(nil), nil

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

func parseTask(s string, args Args) (Task, error) {
	parts := regexp.MustCompile("\\s+").Split(s, -1)

	if args.Check != "" {
		if len(parts) != 2 {
			return Task{}, fmt.Errorf("Incorrect line format")
		}

		path := parts[1]
		hashHex := strings.Replace(parts[0], ":", "", -1)

		hash, err := hex.DecodeString(hashHex)
		if err != nil {
			return Task{}, fmt.Errorf("Unable to decode hex: %s", hashHex)
		}

		if args.Sha256 {
			if len(hash) != 32 {
				return Task{}, fmt.Errorf("Hash should have 32 bytes")
			}

			return Task{path, hash}, nil
		} else {
			if len(hash) != 8 {
				return Task{}, fmt.Errorf("Hash should have 8 bytes")
			}

			return Task{path, hash}, nil
		}
	} else {
		if len(parts) != 1 {
			return Task{}, fmt.Errorf("Incorrect line format")
		}

		if args.Sha256 {
			return Task{parts[0], make([]byte, 32)}, nil
		} else {
			return Task{parts[0], make([]byte, 8)}, nil
		}
	}

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

	var data []byte
	var err error
	if args.Check != "" {
		f, err := os.Open(args.Check)
		if err != nil {
			panic(fmt.Errorf("Cannot open file at %s", args.Check))
		}
		defer f.Close()
		data, err = io.ReadAll(f)
	} else {
		data, err = io.ReadAll(os.Stdin)
	}

	if err != nil {
		panic(fmt.Errorf("Error reading stdin"))
	}

	lines := Input(strings.Split(string(data), "\n"))
	sort.Sort(&lines)

	for _, l := range lines {

		if l != "" {
			task, err := parseTask(l, args)
			if err != nil {
				panic(err)
			}
			tasks = append(tasks, task)
		}
	}

	if len(tasks) == 0 {

		fmt.Printf("%X\n", 0)
		return
	}

	partSize := len(tasks) / runtime.NumCPU()

	var wg sync.WaitGroup
	differs := false
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

				if args.Check != "" && !reflect.DeepEqual(tasks[idx].hash, ret) {
					fmt.Printf("!! %s\n", tasks[idx].absolutePath)
					differs = true
				} else {
					tasks[idx].hash = ret
				}

			}

		}(tasks[start:end])

		start = end
	}

	wg.Wait()

	if args.Check != "" {
		if differs {
			fmt.Println("Failure")
		} else {
			fmt.Println("Success")
		}
	} else {
		for _, task := range tasks {
			fmt.Printf("%s %s\n", fmtHex(task.hash, args), task.absolutePath)
		}
	}

}
