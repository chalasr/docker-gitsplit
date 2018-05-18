package gitsplit

import (
    "github.com/libgit2/git2go"
    "github.com/pkg/errors"
    log "github.com/sirupsen/logrus"
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
    log.Info("Working on ", repository.Path())

    return &WorkingSpace{
        config: config,
        repository: repository,
        remotes: NewGitRemoteCollection(repository),
    }, nil
}

func (w *WorkingSpaceFactory) getRepository(config Config) (*git.Repository, error) {
    if config.CacheUri.IsLocal() && !utils.FileExists(config.CacheUri.SchemelessUri()) {
        repository, err := git.InitRepository(config.CacheUri.SchemelessUri(), true)
        if err != nil {
            return nil, errors.Wrap(err, "Fail to initialize cache repository")
        }
        repository.Free()
    }
    repoPath := "/tmp/toto"
    if utils.FileExists(repoPath) {
        return git.OpenRepository(repoPath)
    }

    if config.CacheUri.IsLocal() && utils.FileExists(config.CacheUri.SchemelessUri()) {
        if err := utils.Copy(config.CacheUri.SchemelessUri(), repoPath); err != nil {
            return nil, errors.Wrap(err, "Fail to create working space from cache")
        }

        return git.OpenRepository(repoPath)
    }

    if _, err := utils.GitExec(repoPath, "clone", "--mirror", config.CacheUri.Uri(), repoPath); err != nil {
        return nil, errors.Wrap(err, "Fail to create working space from cache")
    }

    return git.OpenRepository(repoPath)
}

func (w *WorkingSpace) Repository() *git.Repository {
    return w.repository
}

func (w *WorkingSpace) Remotes() *GitRemoteCollection {
    return w.remotes
}

func (w *WorkingSpace) Init() error {
    if w.config.CacheUri.IsLocal() && !utils.FileExists(w.config.CacheUri.SchemelessUri()) {
        repository, err := git.InitRepository(w.config.CacheUri.SchemelessUri(), true)
        if err != nil {
            return errors.Wrap(err, "Fail to initialize cache repository")
        }
        repository.Free()
    }
    w.remotes.Add("cache", w.config.CacheUri.Uri(), []string{"split-flag"})
    w.remotes.Flush()

    w.remotes.Add("origin", w.config.ProjectUri.Uri(), []string{"heads", "tags"}).Fetch()

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
    remote, err := w.remotes.Get("cache")
    if err == nil {
        remote.PushMirror()
    }

    if err := w.remotes.Flush(); err != nil {
        log.Fatal(err)
    }
    w.repository.Free()
    //rm repoPath
}
