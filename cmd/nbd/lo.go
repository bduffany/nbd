//go:build linux
// +build linux

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
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"

	"github.com/bduffany/nbd"
	"github.com/bduffany/nbd/nbdnl"
	"github.com/google/subcommands"
	"golang.org/x/sys/unix"
)

func init() {
	commands = append(commands, &loCmd{})
}

type loCmd struct {
	index uint
}

func (cmd *loCmd) Name() string {
	return "lo"
}

func (cmd *loCmd) Synopsis() string {
	return "Provide file locally as a block device"
}

func (cmd *loCmd) Usage() string {
	return `Usage: nbd lo <file>

Provide file locally as a block device. An NBD device node will be chosen automatically and the path of that device printed to stdout.

As a special feature, you can toggle write-only mode by sending a SIGUSR1. In
write-only mode, all write-requests are denied with a EPERM. This is useful for
testing crash-resilience of an application on a given filesystem. You can
create a virtual block device with a filesystem of your choice and have the
application under test write to it. When you want to simulate a crash, you send
a SIGUSR1 and unmount the device. You then send another SIGUSR1 and remount the
filesystem to check whether invariants of the application survived the "crash".
`
}

func (cmd *loCmd) SetFlags(fs *flag.FlagSet) {
	fs.UintVar(&cmd.index, "index", uint(nbdnl.IndexAny), "NBD device index")
}

func (cmd *loCmd) Execute(ctx context.Context, fs *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	// if fs.NArg() != 1 {
	// 	log.Print(cmd.Usage())
	// 	return subcommands.ExitUsageError
	// }

	var wg sync.WaitGroup
	for i := 0; i < fs.NArg(); i++ {
		i := i

		wg.Add(1)
		go func() {
			defer wg.Done()
			execute(ctx, fs.Arg(i), uint32(i)+uint32(cmd.index))
		}()
	}

	wg.Wait()
	return subcommands.ExitSuccess
}

func execute(ctx context.Context, path string, index uint32) subcommands.ExitStatus {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		log.Printf("open %s: %s", path, err)
		return subcommands.ExitFailure
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		log.Printf("stat %s: %s", f.Name(), err)
		return subcommands.ExitFailure
	}
	log.Printf("setting up loopback for %s (%d bytes)", path, fi.Size())

	d := &crashable{Device: f}
	ch := make(chan os.Signal, 16)
	signal.Notify(ch, unix.SIGUSR1)
	go func() {
		for range ch {
			d.toggleCrash()
		}
	}()

	dev, err := nbd.Loopback(ctx, d, uint64(fi.Size()), index)
	if err != nil {
		log.Printf("Failed to set up nbd: %s", err)
		return subcommands.ExitFailure
	}

	disconnected := make(chan struct{})
	interrupt := make(chan os.Signal, 16)
	signal.Notify(interrupt, os.Interrupt)
	go func() {
		for range interrupt {
			log.Printf("Disconnecting /dev/nbd%d", dev.Index)
			if err := nbdnl.Disconnect(dev.Index); err != nil {
				log.Printf("Error while disconnecting /dev/nbd%d: %s", dev.Index, err)
			} else {
				log.Printf("Disconnected /dev/nbd%d", dev.Index)
			}
			close(disconnected)
		}
	}()

	log.Printf("Connected to /dev/nbd%d", dev.Index)
	if err := dev.Wait(); err != nil {
		log.Printf("dev.Wait(): %s", err)
		return subcommands.ExitFailure
	}
	<-disconnected
	return subcommands.ExitSuccess
}

type crashable struct {
	nbd.Device
	crashed uint32
}

func (c *crashable) toggleCrash() {
	if atomic.AddUint32(&c.crashed, 1<<31) == 0 {
		log.Println("SIGUSR1 received, device is read-write")
	} else {
		log.Println("SIGUSR1 received, device is read-only")
	}
}

func (c *crashable) WriteAt(p []byte, offset int64) (n int, err error) {
	if atomic.LoadUint32(&c.crashed) != 0 {
		return 0, nbd.Errorf(nbd.EPERM, "write-only")
	}
	return c.Device.WriteAt(p, offset)
}
