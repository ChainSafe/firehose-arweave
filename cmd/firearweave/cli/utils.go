// Copyright 2021 dfuse Platform Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/streamingfast/cli"
	"github.com/streamingfast/logging"
	"go.uber.org/zap"
)

func mustReplaceDataDir(dataDir, in string) string {
	d, err := filepath.Abs(dataDir)
	if err != nil {
		panic(fmt.Errorf("file path abs: %w", err))
	}

	in = strings.Replace(in, "{data-dir}", d, -1)
	return in
}

func mkdirStorePathIfLocal(storeURL string) (err error) {
	rootLog.Debug("creating directory and its parent(s)", zap.String("directory", storeURL))
	if dirs := getDirsToMake(storeURL); len(dirs) > 0 {
		err = makeDirs(dirs)
	}
	return
}

func getDirsToMake(storeURL string) []string {
	parts := strings.Split(storeURL, "://")
	if len(parts) > 1 {
		if parts[0] != "file" {
			// Not a local store, nothing to do
			return nil
		}
		storeURL = parts[1]
	}

	// Some of the store URL are actually a file directly, let's try our best to cope for that case
	filename := filepath.Base(storeURL)
	if strings.Contains(filename, ".") {
		storeURL = filepath.Dir(storeURL)
	}

	// If we reach here, it's a local store path
	return []string{storeURL}
}

func makeDirs(directories []string) error {
	for _, directory := range directories {
		err := os.MkdirAll(directory, 0755)
		if err != nil {
			return fmt.Errorf("failed to create directory %q: %w", directory, err)
		}
	}

	return nil
}

// MustReplaceDataDir is used in sf-ethereum-priv
func MustReplaceDataDir(dataDir, in string) string {
	d, err := filepath.Abs(dataDir)
	if err != nil {
		panic(fmt.Errorf("file path abs: %w", err))
	}

	in = strings.Replace(in, "{data-dir}", d, -1)
	return in
}

var DefaultLevelInfo = logging.LoggerDefaultLevel(zap.InfoLevel)

func FlagDescription(in string, args ...interface{}) string {
	return fmt.Sprintf(strings.Join(strings.Split(string(cli.Description(in)), "\n"), " "), args...)
}

func countBlocks(dataDir string) (count uint64) {
	count = 0
	oneBlocksDir := dataDir + "/storage/one-blocks"
	mergedBlocksDir := dataDir + "/storage/merged-blocks"

	// count oneBlocks
	oneBlocks, err := ioutil.ReadDir(oneBlocksDir)
	if err == nil {
		count += uint64(len(oneBlocks))
	}

	// count mergedBlocks
	mergedBlocks, err := ioutil.ReadDir(mergedBlocksDir)
	if err == nil {
		count += uint64(len(mergedBlocks)) * 100
	}

	// try get last block number from one blocks and merged blocks
	lastBlock := max(lastBlockIn(oneBlocks), lastBlockIn(mergedBlocks)+100)
	if lastBlock > count {
		count = lastBlock + 1
	}

	return
}

// compute the last block number from filenames
func lastBlockIn(blocks []os.FileInfo) (last uint64) {
	last = 0
	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].Name() < blocks[j].Name()
	})

	blocksLen := len(blocks)
	if blocksLen == 0 {
		return
	}

	// try get last blocks number
	lastBlockFile := blocks[blocksLen-1].Name()
	parts := strings.Split(lastBlockFile, "-")
	if len(parts) > 0 {
		lastBlock, err := strconv.ParseUint(parts[0], 10, 64)
		if err == nil && lastBlock > last {
			last = lastBlock
		}
	}

	return
}

// returns the larger of x or y.
func max(x, y uint64) uint64 {
	if x < y {
		return y
	}

	return x
}
