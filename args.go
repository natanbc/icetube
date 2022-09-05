package main

import (
    "flag"
    "fmt"
    "os"
    "path/filepath"
)

var _fs *flag.FlagSet
func init() {
    _fs = flag.NewFlagSet("flags", flag.ExitOnError)
    _fs.Usage = printUsage
    _fs.BoolVar(&keepAac, "keep-aac", false, "Don't transcode youtube AAC to opus before sending to icecast.")
}

func parseArgs() (string, string) {
    flag.Parse()
    args := flag.Args()
    if len(args) != 2 {
        printUsage()
        os.Exit(1)
    }
    return args[0], args[1]
}

func printUsage() {
    self := filepath.Base(os.Args[0])
    fmt.Fprintf(os.Stderr, `
Usage: %[1]s [OPTIONS] <youtube video link/id> <icecast server URL>

Options:
    --keep-aac
        Don't convert the youtube audio to opus. AAC might work with icecast,
        but is not supported.

Examples:
    %[1]s https://youtu.be/jfKfPfyJRdk icecast://source:PASSWORD@my-server:8001/stream.ogg
`, self)
}

