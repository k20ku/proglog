package main

import (
	"flag"
	"log"
	"os"

	api "github.com/k20ku/proglog/gen/go/log/v1"
	lg "github.com/k20ku/proglog/internal/log"
)

func run(name string, n int, offset uint64) error {
	c := lg.Config{}
	l, err := lg.NewLog(name, c)
	if err != nil {
		return err
	}
	defer l.Close()
	for range n {
		if _, err = l.Append(
			&api.Record{Value: []byte("Hello Proglog!")},
		); err != nil {
			return err
		}
	}
	off, err := l.HighestOffset()
	if err != nil {
		return err
	}
	log.Println(off)
	record, err := l.Read(off)
	if err != nil {
		return err
	}
	log.Print(record.Offset, ": highest offset")
	record, err = l.Read(offset)
	if err != nil {
		return err
	}
	log.Print(record.Offset, ": read offset")
	return nil
}
func main() {
	trunc := flag.Bool("trunc", false, "a bool: whether you truc tmp")
	n := flag.Int("n", 1, "a int: number of writing to the log")
	offset := flag.Uint64("offset", 0, "a uint64: offset at which you want to read")
	flag.Parse()

	pwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	path := pwd + "/tmp/log/data"

	if *trunc {
		if err := os.RemoveAll(path); err != nil {
			log.Fatal(err)
		}
	}
	if err := os.MkdirAll(path, 0750); err != nil {
		log.Fatal(err)
	}
	if err := run(path, *n, *offset); err != nil {
		log.Fatal(err)
	}
}
