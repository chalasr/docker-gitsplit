package gitsplit

import (
    "github.com/libgit2/git2go"
    "github.com/pkg/errors"
    "github.com/jderusse/gitsplit/utils"
)

type WorkingSpaceFactory struct {
}

type WorkingSpace struct {
    config     Config
    repository *git.Repository
    remotes    *GitRemoteCollection
}

func NewWorkingSpaceFactory() *WorkingSpaceFactory {
    return &WorkingSpaceFactory {
    }
}

func (w *WorkingSpaceFactory) CreateWorkingSpace(config Config) (*WorkingSpace, error) {
    repository, err := w.getRepository(config)
    if err != nil {
        return nil, errors.Wrap(err, "Fail to create working repository")
    }

    return &WorkingSpace{
        config: config,
        repository: repository,
        remotes: NewGitRemoteCollection(repository),
    }, nil
}

func (w *WorkingSpaceFactory) getRepository(config Config) (*git.Repository, error) {
    if utils.FileExists(config.CacheDir+"/.git") {
        return git.OpenRepository(config.CacheDir)
    }

    if utils.FileExists(config.CacheDir) {
        return git.OpenRepository(config.CacheDir)
    }

    return git.InitRepository(config.CacheDir, true)
}

func (w *WorkingSpace) Repository() *git.Repository {
    return w.repository
}

func (w *WorkingSpace) Remotes() *GitRemoteCollection {
    return w.remotes
}

func (w *WorkingSpace) Init() error {

    w.remotes.Add("cache", "", []string{"split-flag"})
    w.remotes.Add("origin", w.config.ProjectDir, []string{"heads", "tags"}).Fetch()

    for _, split := range w.config.Splits {
        for _, target := range split.Targets {
            remote := w.remotes.Add(target, target, []string{"heads", "tags"})
            remote.Fetch()
        }
    }
    go w.remotes.Clean()

    if err := w.remotes.Flush(); err != nil {
        return err
    }

    return nil
}

func (w *WorkingSpace) Close() {
    w.remotes.Flush()
    w.repository.Free()
}
