package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"runtime"
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

var args struct {
	Strict bool
	Colon  bool
	Sha256 bool
}

func check(path string, err error) {
	if err != nil {
		fmt.Printf("%s: %v\n", path, err)

		if args.Strict {
			panic(err)
		}
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

func getHashForFile(path string, buffer *[]byte, isSha bool) []byte {

	f, err := os.Open(path)

	check(path, err)

	defer f.Close()

	var size int
	info, err := f.Stat()

	check(path, err)

	size64 := info.Size()
	if int64(int(size64)) == size64 {
		size = int(size64)
	}

	size++

	if size > cap(*buffer) {
		*buffer = make([]byte, size)
	}

	ind := 0
	for {
		n, err := f.Read((*buffer)[ind:])
		ind += n

		if err != nil {
			if err == io.EOF {
				break
			}

			check(path, err)
		}
	}

	if isSha {
		h := sha256.New()
		h.Write((*buffer)[:size])
		return h.Sum(nil)
	} else {
		return uint64ToBytes(wyhash.Sum64((*buffer)[:size]))
	}
}

func main() {

	arg.MustParse(&args)

	var tasks []Task

	data, err := io.ReadAll(os.Stdin)

	if err != nil {
		panic(fmt.Errorf("Error reading stdin"))
	}

	lines := strings.Split(string(data), "\n")

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
			var threadLocalBuffer []byte

			for _, idx := range batches[index] {
				tasks[idx].hash = getHashForFile(tasks[idx].absolutePath, &threadLocalBuffer, args.Sha256)
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
