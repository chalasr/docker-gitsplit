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

func handleError(err error) {
    log.Fatal(err)
}

func main() {
    flag.Parse()

    config, err := gitsplit.NewConfigFromFile(".gitsplit.yml")
    if err != nil {
        handleError(err)
    }

    workingSpaceFactory := gitsplit.NewWorkingSpaceFactory()

    workingSpace, err := workingSpaceFactory.CreateWorkingSpace(*config)
    defer workingSpace.Close()
    if err != nil {
        handleError(err)
    }
    if err := workingSpace.Init(); err != nil {
        handleError(err)
    }

    splitter := gitsplit.NewSplitter(*config, workingSpace)
    if err := splitter.Split(whitelistReferences); err != nil {
        handleError(err)
    }
    log.Info("Done")
}
