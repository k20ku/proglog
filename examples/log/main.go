package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path"

	api "github.com/k20ku/proglog/gen/go/log/v1"
	logvx "github.com/k20ku/proglog/internal/log"
)

func run(dir string, n int, offset uint64) error {
	c := logvx.Config{
		Segment: logvx.SegmentConfig{
			MaxStoreBytes: 1024,
			MaxIndexBytes: 1024,
		},
	}
	l, err := logvx.NewLog(dir, c)
	if err != nil {
		return err
	}
	defer func() { _ = l.Close() }()
	if n < 0 {
		record, err := l.Read(offset)
		if err != nil {
			return handleRead(offset, err)
		}
		fmt.Printf("\noffset %d:\n %#v\n", offset, record)
	} else {
		for range n {
			if _, err = l.Append(
				&api.Record{Value: []byte("Hello Proglog!")},
			); err != nil {
				return err
			}
		}
	}
	{
		off, err := l.HighestOffset()
		if err != nil {
			return err
		}
		record, err := l.Read(off)
		if err != nil {
			return handleRead(off, err)
		}
		fmt.Printf("\ncurrent highest offset: %d\n", record.Offset)
	}
	return nil
}

func handleRead(off uint64, err error) error {
	if err, ok := errors.AsType[logvx.ErrOffsetOutOfRange](err); ok {
		fmt.Printf("\n%+v\n", err)
		return nil
	}
	return fmt.Errorf("read offset %d: %w", off, err)
}

func main() {
	trunc := flag.Bool("trunc", false, "a bool: whether you truc tmp")
	n := flag.Int("n", -1, "a int: number of writing to the log")
	offset := flag.Uint64("offset", 0, "a uint64: offset at which you want to read")
	flag.Parse()

	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	dir := path.Join(home, ".proglog", "example", "log", "wal-log")
	fmt.Print("log data dir: ", dir)

	if *trunc {
		if err := os.RemoveAll(dir); err != nil {
			log.Fatal(err)
		}
	}
	if err := os.MkdirAll(dir, 0750); err != nil {
		log.Fatal(err)
	}
	if err := run(dir, *n, *offset); err != nil {
		log.Fatal(err)
	}
}
