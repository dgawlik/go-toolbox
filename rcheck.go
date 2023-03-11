package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/gobwas/glob"
	"github.com/orisano/wyhash"
	"github.com/pelletier/go-toml/v2"
)

const SEED = 1
const MAX_BATCH = 24

type Task struct {
	absolutePath string
	hash         uint64
}

type Batch struct {
	taskIndex []int
}

type Config struct {
	Roots               []string
	Excludes            []string
	Cores               int
	FollowSymlinks      bool
	SaveDetailsSnapshot bool
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

var configLocation string
var diffSource string

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

func exhaustRoot(root string, cfg Config) ([]Task, error) {
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

	if err != nil {
		return nil, err
	}

	return files, nil
}

func getHash(path string) uint64 {
	s, err := os.ReadFile(path)

	check(err)

	return wyhash.Sum64(1, append(s, []byte(path)...))
}

func resolveFile(path string, followSymlinks bool) string {
	info, err := os.Lstat(path)

	if err != nil {
		panic(err)
	}

	if info.Mode()&fs.ModeSymlink != 0 && followSymlinks {
		targetPath, err := filepath.EvalSymlinks(path)

		if err != nil {
			panic(err)
		}

		return resolveFile(targetPath, followSymlinks)
	}

	return path
}

func main() {

	flag.StringVar(&configLocation, "config", "./config.toml", "--config <config location>")
	flag.StringVar(&diffSource, "diff", "", "--diff <source location>")

	flag.Parse()

	s, err := os.ReadFile(configLocation)
	check(err)

	var cfg Config
	err = toml.Unmarshal([]byte(s), &cfg)
	check(err)

	var tasks []Task
	for _, root := range cfg.Roots {
		part, err := exhaustRoot(root, cfg)
		check(err)
		tasks = append(tasks, part...)
	}

	if len(tasks) == 0 {
		fmt.Printf("%X\n", 0)
		return
	}

	var batches []Batch

	partSize := (len(tasks) + MAX_BATCH - 1) / MAX_BATCH
	for it := 0; it < len(tasks); {

		end := it + partSize
		if it+partSize > len(tasks) {
			end = len(tasks)
		}

		var idx []int
		for i := it; i < end; i++ {
			idx = append(idx, i)
		}

		batches = append(batches, Batch{idx})
		it = end
	}

	var wg sync.WaitGroup
	for i := 0; i < len(batches); i++ {
		wg.Add(1)

		go func(index int) {
			defer wg.Done()

			for _, idx := range batches[index].taskIndex {
				tasks[idx].hash = getHash(tasks[idx].absolutePath)
			}
		}(i)
	}

	wg.Wait()

	var sb strings.Builder
	for _, task := range tasks {
		s := fmt.Sprintf("%s %X\n", task.absolutePath, task.hash)
		sb.WriteString(s)
	}

	if cfg.SaveDetailsSnapshot {
		snapFile, err := os.Create(".snapshot")
		check(err)
		fmt.Fprint(snapFile, sb.String())
		err = snapFile.Close()
		check(err)
	}

	if diffSource == "" {
		total := sb.String()

		result := wyhash.Sum64(1, []byte(total))

		fmt.Printf("%s\n", fmtHex(result))
	} else {
		diff, err := os.ReadFile(diffSource)
		check(err)

		diffStr := string(diff)

		lines := strings.Split(diffStr, "\n")

		orig := make(map[string]Task)
		for _, t := range tasks {
			orig[t.absolutePath] = t
		}

		dif := make(map[string]uint64)
		for _, l := range lines {
			pair := strings.Split(l, " ")
			if len(pair) == 2 {
				i, err := strconv.ParseUint(pair[1], 16, 0)
				check(err)
				dif[pair[0]] = i
			}
		}

		var added []Task
		var removed []Task
		var changed []Task

		for k, v := range orig {
			_, ok := dif[k]

			if !ok {
				added = append(added, Task{k, v.hash})
			}
		}

		for k, v := range dif {
			v2, ok := orig[k]

			if !ok {
				removed = append(removed, Task{k, v2.hash})
			} else if v2.hash != v {
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

}
