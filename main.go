package main

import (
    "flag"
    "strings"
    log "github.com/sirupsen/logrus"
    "github.com/jderusse/gitsplit/gitsplit"
)

type arrayFlags []string

func (i *arrayFlags) String() string {
    return strings.Join(*i, ", ")
}

func (i *arrayFlags) Set(value string) error {
    *i = append(*i, value)
    return nil
}

var whitelistReferences arrayFlags

func init() {
    flag.Var(&whitelistReferences, "ref", "References to split.")
}

func main() {
    flag.Parse()

    config := gitsplit.NewConfigFromFile(".gitsplit.yml")
    splitter, err := gitsplit.NewSplitter(config)
    if err != nil {
        log.Fatal(err)
    }

    defer splitter.Close()

    if err := splitter.Split(whitelistReferences); err != nil {
        log.Fatal(err)
    }

    log.Info("Done")
}
