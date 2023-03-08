package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/gobwas/glob"
	"github.com/orisano/wyhash"
	"github.com/pelletier/go-toml/v2"
)

const SEED = 1
const MAX_BATCH = 24

type Task struct {
	absolutePath string
}

type Batch struct {
	tasks []Task
	hash  uint64
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

			if err != nil {
				panic(err)
			}

			if info.Mode().IsRegular() {
				if cfg.Verbose {
					fmt.Printf("%s\n", newPath)
				}

				files = append(files, Task{newPath})
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

		batches = append(batches, Batch{tasks[it:end], 0})
		it = end
	}

	var wg sync.WaitGroup
	for i := 0; i < len(batches); i++ {
		wg.Add(1)

		i := i
		go func() {
			defer wg.Done()

			work := batches[i]

			var secondLevel []byte
			for _, task := range work.tasks {
				hash := getHash(task.absolutePath)

				hashBytes := make([]byte, 8)
				binary.LittleEndian.PutUint64(hashBytes, hash)
				secondLevel = append(secondLevel, hashBytes...)
			}

			work.hash = wyhash.Sum64(1, secondLevel)
		}()
	}

	wg.Wait()

	var topLevel []byte
	for _, batch := range batches {
		hashBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(hashBytes, batch.hash)
		topLevel = append(topLevel, hashBytes...)
	}

	result := wyhash.Sum64(1, topLevel)

	fmt.Printf("%X\n", result)
}
