package gitsplit

import (
    "github.com/libgit2/git2go"
)

type ReferenceSplitterInterface interface {
    Split(reference string, prefixes []string) (*git.Oid, error)
}

type ReferenceSplitterFactoryInterface interface {
    New(repository *git.Repository) ReferenceSplitterInterface
}
