//go:build linux || darwin
// +build linux darwin

// Copyright 2018 Axel Wagner
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"os"

	"github.com/bduffany/nbd"
	"golang.org/x/sys/unix"
)

func blockSize(fi os.FileInfo) *nbd.BlockSizeConstraints {
	if st, ok := fi.Sys().(*unix.Stat_t); ok {
		if st.Blksize > 0xffff {
			return nil
		}
		return &nbd.BlockSizeConstraints{
			Min:       1,
			Preferred: uint32(st.Blksize),
			Max:       0xffff,
		}
	}
	return nil
}
