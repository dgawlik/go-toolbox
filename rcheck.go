package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
	Roots          []string
	Excludes       []string
	Cores          int
	FollowSymlinks bool
	Verbose        bool
}

func check(e error) {
	if e != nil {
		panic(e)
	}
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

func readArgs(args []string) (string, error) {
	if len(args) == 0 {
		return "./config.toml", nil
	} else if len(args) == 2 && args[0] == "--config" {
		return args[1], nil
	} else {
		return "", errors.New("Usage: prog [--config <path>]")
	}
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

	args := os.Args[1:]
	configLocation, err := readArgs(args)
	check(err)

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

		if cfg.Verbose {
			log := s
			fmt.Print(log)
		}

		sb.WriteString(s)
	}

	total := sb.String()

	result := wyhash.Sum64(1, []byte(total))

	fmt.Printf("\n%X\n", result)
}
