package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/gobwas/glob"
	"github.com/pelletier/go-toml/v2"
	"github.com/zhangyunhao116/wyhash"
)

type Task struct {
	absolutePath string
	hash         uint64
}

type Batch []int

type Config struct {
	Roots               []string
	Excludes            []string
	Cores               int
	FollowSymlinks      bool
	SaveDetailsSnapshot bool
}

type RuntimeOptions struct {
	configLocation     string
	diffSourceLocation string
}

var config Config
var runtimeOpts RuntimeOptions

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func fmtHex(num uint64) string {
	parts := make([]byte, 8)
	binary.LittleEndian.PutUint64(parts, num)

	var sb strings.Builder
	for i, b := range parts {
		sb.WriteString(fmt.Sprintf("%X", b))

		if i < 7 {
			sb.WriteString(":")
		}
	}

	return sb.String()
}

func matchesExclude(path string, excludes []string) bool {
	full, base := path, filepath.Base(path)

	for _, patt := range excludes {
		g := glob.MustCompile(patt)

		if g.Match(full) || g.Match(base) {
			return true
		}
	}

	return false
}

func exhaustRootDirectory(root string, cfg Config) ([]Task, error) {
	var files []Task

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			fmt.Printf("%q: %v\n", path, err)
			return err
		}

		if !matchesExclude(path, cfg.Excludes) {
			newPath := resolveFile(path, cfg.FollowSymlinks)

			info, err := os.Lstat(newPath)

			check(err)

			if info.Mode().IsRegular() {
				files = append(files, Task{newPath, 0})
			}
		}
		return nil
	})

	check(err)

	return files, nil
}

func getHashForFile(path string, buffer *[]byte) uint64 {
	f, err := os.Open(path)
	check(err)
	defer f.Close()

	var size int
	if info, err := f.Stat(); err == nil {
		size64 := info.Size()
		if int64(int(size64)) == size64 {
			size = int(size64)
		}
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
			panic(err)
		}
	}

	return wyhash.Sum64((*buffer)[:size])
}

func resolveFile(path string, followSymlinks bool) string {
	info, err := os.Lstat(path)

	check(err)

	if info.Mode()&fs.ModeSymlink != 0 && followSymlinks {
		targetPath, err := filepath.EvalSymlinks(path)

		check(err)

		return resolveFile(targetPath, followSymlinks)
	}

	return path
}

func printDiff(prevList map[string]Task, currList map[string]Task) {
	var added []Task
	var removed []Task
	var changed []Task

	for k, v := range prevList {
		_, ok := currList[k]

		if !ok {
			added = append(added, Task{k, v.hash})
		}
	}

	for k, v := range currList {
		v2, ok := prevList[k]

		if !ok {
			removed = append(removed, Task{k, v2.hash})
		} else if v2.hash != v.hash {
			changed = append(changed, Task{k, v2.hash})
		}
	}

	for _, t := range added {
		fmt.Printf("+%s %X\n", t.absolutePath, t.hash)
	}

	for _, t := range removed {
		fmt.Printf("-%s %X\n", t.absolutePath, t.hash)
	}

	for _, t := range changed {
		fmt.Printf("~%s %X\n", t.absolutePath, t.hash)
	}
}

func main() {

	// f, err := os.Create(".profile")
	// pprof.StartCPUProfile(f)
	// defer pprof.StopCPUProfile()

	flag.StringVar(&runtimeOpts.configLocation, "config", "./config.toml", "--config <config location>")
	flag.StringVar(&runtimeOpts.diffSourceLocation, "diff", "", "--diff <source location>")

	flag.Parse()

	s, err := os.ReadFile(runtimeOpts.configLocation)
	check(err)

	err = toml.Unmarshal([]byte(s), &config)
	check(err)

	var tasks []Task
	for _, root := range config.Roots {
		part, err := exhaustRootDirectory(root, config)
		check(err)
		tasks = append(tasks, part...)
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
				tasks[idx].hash = getHashForFile(tasks[idx].absolutePath, &threadLocalBuffer)
			}
		}(i)
	}

	wg.Wait()

	var sb strings.Builder
	for _, task := range tasks {
		s := fmt.Sprintf("%s %X\n", task.absolutePath, task.hash)
		sb.WriteString(s)
	}

	if config.SaveDetailsSnapshot {
		snapFile, err := os.Create(".snapshot")
		check(err)
		fmt.Fprint(snapFile, sb.String())
		err = snapFile.Close()
		check(err)
	}

	if runtimeOpts.diffSourceLocation == "" {
		total := sb.String()

		result := wyhash.Sum64([]byte(total))

		fmt.Printf("%s\n", fmtHex(result))
	} else {
		diff, err := os.ReadFile(runtimeOpts.diffSourceLocation)
		check(err)

		diffStr := string(diff)

		lines := strings.Split(diffStr, "\n")

		prevList := make(map[string]Task)
		for _, t := range tasks {
			prevList[t.absolutePath] = t
		}

		currList := make(map[string]Task)
		for _, l := range lines {
			pair := strings.Split(l, " ")
			if len(pair) == 2 {
				i, err := strconv.ParseUint(pair[1], 16, 0)
				check(err)
				currList[pair[0]] = Task{pair[0], i}
			}
		}

		printDiff(prevList, currList)
	}
}
