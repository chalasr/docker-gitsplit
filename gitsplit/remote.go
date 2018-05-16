package gitsplit

import (
    "os"
    "sync"
    "regexp"
    "strings"
    log "github.com/sirupsen/logrus"
    "github.com/pkg/errors"
    "github.com/gosimple/slug"
    "github.com/libgit2/git2go"
    "github.com/jderusse/gitsplit/utils"
)

type GitRemoteCollection struct {
    repository *git.Repository
    items      map[string]*GitRemote
}

func NewGitRemoteCollection(repository *git.Repository) *GitRemoteCollection {
    return &GitRemoteCollection{
        items: make(map[string]*GitRemote),
        repository: repository,
    }
}

func (r *GitRemoteCollection) Add(alias string, url string) *GitRemote {
    remote := NewGitRemote(r.repository, alias, url)
    r.items[alias] = remote

    return remote
}

func (r *GitRemoteCollection) Get(alias string) (*GitRemote, error) {
    if remote, ok := r.items[alias]; !ok {
        return nil, errors.New("The remote does not exists")
    } else {
        return remote, nil
    }

}

func (r *GitRemoteCollection) Clean() {
    knownRemotes := []string{}
    for _, remote := range r.items {
        knownRemotes = append(knownRemotes, remote.id)
    }

    mutexRemoteList.Lock()
    defer mutexRemoteList.Unlock()
    remotes, err := r.repository.Remotes.List()
    if err != nil {
        return
    }

    for _, remoteId := range remotes {
        if !utils.InArray(knownRemotes, remoteId) {
            log.Info("Removing remote ", remoteId)
            r.repository.Remotes.Delete(remoteId)
        }
    }
}

func (r *GitRemoteCollection) Flush() error {
    for _, remote := range r.items {
        if err := remote.Flush(); err != nil {
            return err
        }
    }

    return nil
}

var mutexRemoteList = &sync.Mutex{}
var mutexReferences = &sync.Mutex{}

type GitRemote struct {
    repository      *git.Repository
    id              string
    alias           string
    url             string
    pool            *utils.Pool
    cacheReferences []Reference
}

func NewGitRemote(repository *git.Repository, alias string, url string) *GitRemote {
    id := slug.Make(alias)
    if id != alias {
        id = id+"-"+utils.Hash(alias)
    }

    return &GitRemote {
        repository: repository,
        id: id,
        alias: alias,
        url: url,
        pool: utils.NewPool(10),
    }
}

func (r *GitRemote) init() error {
    mutexRemoteList.Lock()
    defer mutexRemoteList.Unlock()

    remotes, err := r.repository.Remotes.List()
    if err != nil {
        return err
    }

    if !utils.InArray(remotes, r.id) {
        if _, err := r.repository.Remotes.Create(r.id, os.ExpandEnv(r.url)); err != nil {
            return errors.Wrapf(err, "Fail to create remote %s", r.alias)
        }
    } else {
        if err := r.repository.Remotes.SetUrl(r.id, os.ExpandEnv(r.url)); err != nil {
            return errors.Wrapf(err, "Fail to update remote %s", r.alias)
        }
    }

    return nil
}

func (r *GitRemote) GetReferences() ([]Reference, error) {
    if r.cacheReferences != nil {
        return r.cacheReferences, nil
    }

    mutexRemoteList.Lock()
    defer mutexRemoteList.Unlock()
    result, err := utils.GitExec(r.repository.Path(), "ls-remote", r.id)
    if err != nil {
        return nil, errors.Wrap(err, "Fail to fetch references")
    }

    references := []Reference{}
    cleanRegexp := regexp.MustCompile("^refs/(tags|heads)/")
    for _, line := range strings.Split(result.Output, "\n") {
        if len(line) == 0 {
            continue
        }
        columns := strings.Split(line, "\t")
        if len(columns) != 2 {
            return nil, errors.New("Fail to parse reference, 2 columns expected. Got " + line)
        }
        referenceId := columns[0];
        referenceName := columns[1];
        oid, err := git.NewOid(referenceId)
        if err != nil {
            return nil, errors.Wrapf(err, "Fail to parse reference %s", line)
        }
        references = append(references, Reference{
            ShortName: cleanRegexp.ReplaceAllString(referenceName, ""),
            Name: referenceName,
            Id:   oid,
        })
    }
    r.cacheReferences = references

    return r.cacheReferences, nil
}

func (r *GitRemote) Fetch() {
    r.init()
    r.pool.Push(func() (interface{}, error) {
        log.Info("Fetching from remote ", r.alias)
        if _, err := utils.GitExec(r.repository.Path(), "fetch", "-p", r.id); err != nil {
            return nil, errors.Wrapf(err, "Fail to update cache of %s", r.alias)
        }

        if _, err := utils.GitExec(r.repository.Path(), "fetch", "--tags", r.id); err != nil {
            return nil, errors.Wrapf(err, "Fail to update cache of %s", r.alias)
        }

        return nil, nil
    })
}

func (r *GitRemote) Push(reference Reference, splitId *git.Oid, target string) {
    r.init()
    r.pool.Push(func() (interface{}, error) {
        references, err := r.GetReferences()
        if err != nil {
            return nil, errors.Wrapf(err, "Fail to get references for remote %s", r.alias)
        }

        for _, remoteReference := range references {
            if remoteReference.Name == reference.Name {
                if remoteReference.Id.Equal(splitId) {
                    log.Info("Already pushed "+reference.ShortName+" into "+target)
                    return nil, nil
                }
                log.Warn("Out of date "+reference.ShortName+" into "+target)
                break
            }
        }

        log.Warn("Pushing "+reference.ShortName+" into "+target)
        mutexRemoteList.Lock()
        r.cacheReferences = nil
        mutexRemoteList.Unlock()
        if _, err := utils.GitExec(r.repository.Path(), "push", "-f", r.id, splitId.String()+":"+reference.Name); err != nil {
            return nil, errors.Wrap(err, "Fail to push")
        }

        mutexRemoteList.Lock()
        r.cacheReferences = nil
        mutexRemoteList.Unlock()
        return nil, nil
    })
}

func (r *GitRemote) Flush() error {
    results := r.pool.Wait()
    if err := results.FirstError(); err != nil {
        return err
    }

    return nil
}
