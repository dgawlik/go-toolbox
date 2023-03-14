package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"runtime"
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

type Batch []int

type Input []string

var args struct {
	Colon  bool
	Sha256 bool
}

func check(path string, err error) {
	if err != nil {
		fmt.Printf("%s: %v\n", path, err)

		panic(err)
	}
}

func uint64ToBytes(num uint64) []byte {
	bt := make([]byte, 8)

	binary.LittleEndian.PutUint64(bt, num)
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

func getHashForFile(path string, isSha bool) []byte {

	buffer, err := os.ReadFile(path)

	check(path, err)

	if isSha {
		h := sha256.New()
		h.Write(buffer)
		return h.Sum(nil)
	} else {
		h := wyhash.NewDefault()
		h.Write(buffer)
		return uint64ToBytes(h.Sum64())
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

	var batches []Batch

	partSize := (len(tasks) + runtime.NumCPU() - 1) / runtime.NumCPU()
	for it := 0; it < len(tasks); {

		end := it + partSize
		if it+partSize > len(tasks) {
			end = len(tasks)
		}

		var idx []int
		for i := it; i < end; i++ {
			idx = append(idx, i)
		}

		batches = append(batches, Batch(idx))
		it = end
	}

	var wg sync.WaitGroup
	for i := 0; i < len(batches); i++ {
		wg.Add(1)

		go func(index int) {
			defer wg.Done()

			for _, idx := range batches[index] {
				tasks[idx].hash = getHashForFile(tasks[idx].absolutePath, args.Sha256)
			}
		}(i)
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
