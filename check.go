package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/alexflint/go-arg"
	"github.com/zeebo/xxh3"
	"github.com/zhangyunhao116/wyhash"
	"github.com/zhenjl/cityhash"
)

type Task struct {
	absolutePath string
	hash         []byte
}

type Batch struct {
	Tasks      []Task
	FileBuffer []byte
}

type Input []string

type Args struct {
	Colon  bool
	Sha256 bool
	Xxh3   bool
	City   bool
}

var args Args

func check(path string, err error) {
	if err != nil {
		fmt.Printf("%s: %v\n", path, err)

		panic(err)
	}
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

func getHashForFile(path string, buffer *[]byte, args Args) []byte {

	f, err := os.Open(path)
	check(path, err)
	defer f.Close()

	info, err := f.Stat()
	check(path, err)
	size := int(info.Size()) + 1

	if size > cap(*buffer) {
		*buffer = make([]byte, size)
	}

	data := (*buffer)[:0]

	err = nil
	for {
		n, err := f.Read(data[len(data):cap(data)])
		data = data[:len(data)+n]
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			break
		}
	}

	check(path, err)

	if args.Sha256 {
		h := sha256.New()
		h.Write(data)
		return h.Sum(nil)
	} else if args.Xxh3 {
		h := xxh3.HashSeed(data, 1)
		return uint64ToBytes(h)
	} else if args.City {
		h := cityhash.CityHash64WithSeed(data, uint32(len(data)), 1)
		return uint64ToBytes(h)
	} else {
		h := wyhash.NewDefault()
		h.Write(data)
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

		batch := Batch{tasks[it:end], make([]byte, 4096)}

		batches = append(batches, batch)
		it = end
	}

	var wg sync.WaitGroup
	for i := 0; i < len(batches); i++ {
		wg.Add(1)

		go func(b *Batch) {
			defer wg.Done()

			for idx, _ := range b.Tasks {
				b.Tasks[idx].hash = getHashForFile(b.Tasks[idx].absolutePath, &b.FileBuffer, args)
			}
		}(&batches[i])
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
