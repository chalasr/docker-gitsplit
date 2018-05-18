package gitsplit

import (
    "os"
    "fmt"
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
    repository      *git.Repository
    items           map[string]*GitRemote
    mutexRemoteList *sync.Mutex
}

func NewGitRemoteCollection(repository *git.Repository) *GitRemoteCollection {
    return &GitRemoteCollection{
        items: make(map[string]*GitRemote),
        repository: repository,
        mutexRemoteList: &sync.Mutex{},
    }
}

func (r *GitRemoteCollection) Add(alias string, url string, refs []string) *GitRemote {
    remote := NewGitRemote(r.repository, alias, url, refs)
    r.items[alias] = remote

    r.mutexRemoteList.Lock()
    defer r.mutexRemoteList.Unlock()
    remote.Init()

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

    r.mutexRemoteList.Lock()
    defer r.mutexRemoteList.Unlock()

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

type GitRemote struct {
    repository      *git.Repository
    id              string
    alias           string
    refs            []string
    url             string
    pool            *utils.Pool
    cacheReferences []Reference
    mutexReferences *sync.Mutex
}

func NewGitRemote(repository *git.Repository, alias string, url string, refs []string) *GitRemote {
    id := slug.Make(alias)
    if id != alias {
        id = id+"-"+utils.Hash(alias)
    }

    return &GitRemote {
        repository: repository,
        id: id,
        alias: alias,
        refs: refs,
        url: url,
        pool: utils.NewPool(10),
        mutexReferences: &sync.Mutex{},
    }
}

func (r *GitRemote) Init() error {
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

func (r *GitRemote) GetReference(alias string) (*Reference, error) {
    references, err := r.GetReferences()
    if err != nil {
        return nil, errors.Wrap(err, "Unable to get reference")
    }

    for _, reference := range references {
        if reference.Alias == alias {
            return &reference, nil
        }
    }

    return nil, nil
}

func (r *GitRemote) AddReference(alias string, id *git.Oid) error {
    r.mutexReferences.Lock()
    defer r.mutexReferences.Unlock()

    r.cacheReferences = nil
    for _, ref := range r.refs {
        reference, err := r.repository.References.Create(fmt.Sprintf("refs/remotes/%s/%s/%s", r.id, ref, alias), id, true, "")
        if err != nil {
            return errors.Wrap(err, "Fail to add reference")
        }
        defer reference.Free()
    }

    return nil
}

func (r *GitRemote) GetReferences() ([]Reference, error) {
    r.mutexReferences.Lock()
    defer r.mutexReferences.Unlock()

    if r.cacheReferences != nil {
        return r.cacheReferences, nil
    }

    iterator, err := r.repository.NewReferenceIteratorGlob(fmt.Sprintf("refs/remotes/%s/*", r.id))
    if err != nil {
        return nil, errors.Wrap(err, "Fail to fetch references")
    }

    defer iterator.Free()
    references := []Reference{}

    reference, err := iterator.Next()
    cleanShortNameRegexp := regexp.MustCompile(fmt.Sprintf("^refs/remotes/%s/", r.id))
    cleanAliasRegexp := regexp.MustCompile(fmt.Sprintf("^refs/remotes/%s/(%s)/", r.id, strings.Join(r.refs, "|")))
    filterRegexp := regexp.MustCompile(fmt.Sprintf("^refs/remotes/%s/(%s)/", r.id, strings.Join(r.refs, "|")))
    for err == nil {
        if filterRegexp.MatchString(reference.Name()) {
            references = append(references, Reference{
                Alias: cleanAliasRegexp.ReplaceAllString(reference.Name(), ""),
                ShortName: cleanShortNameRegexp.ReplaceAllString(reference.Name(), ""),
                Name: reference.Name(),
                Id:   reference.Target(),
            })
        }
        reference, err = iterator.Next()
    }

    r.cacheReferences = references
    return references, nil
}

func (r *GitRemote) Fetch() {
    r.pool.Push(func() (interface{}, error) {
        log.Info("Fetching from remote ", r.alias)

        for _, ref := range (r.refs) {
            if _, err := utils.GitExec(r.repository.Path(), "fetch", "--force", "--prune", r.id, fmt.Sprintf("refs/%s/*:refs/remotes/%s/%s/*", ref, r.id, ref)); err != nil {
                return nil, errors.Wrapf(err, "Fail to update cache of %s", r.alias)
            }
        }

        return nil, nil
    })
}

func (r *GitRemote) PushRef(refs string) {
    r.pool.Push(func() (interface{}, error) {
        log.Warn("Pushing "+refs+" into "+r.alias)
        if _, err := utils.GitExec(r.repository.Path(), "push", "-f", r.id, refs); err != nil {
            return nil, errors.Wrap(err, "Fail to push")
        }

        return nil, nil
    })
}

func (r *GitRemote) PushMirror() {
    r.pool.Push(func() (interface{}, error) {
        log.Warn("Pushing --mirror into "+r.alias)
        if _, err := utils.GitExec(r.repository.Path(), "push", "-f", "--mirror", r.id); err != nil {
            return nil, errors.Wrap(err, "Fail to push")
        }

        return nil, nil
    })
}

func (r *GitRemote) Push(reference Reference, splitId *git.Oid) error {
    references, err := r.GetReferences()
    if err != nil {
        return errors.Wrapf(err, "Fail to get references for remote %s", r.alias)
    }

    for _, remoteReference := range references {
        if remoteReference.Alias == reference.Alias {
            if remoteReference.Id.Equal(splitId) {
                log.Info("Already pushed "+reference.Alias+" into "+r.alias)
                return nil
            }
            log.Warn("Out of date "+reference.Alias+" for "+r.alias)
            break
        }
    }

    r.PushRef(splitId.String()+":refs/"+reference.ShortName)

    return nil
}

func (r *GitRemote) Flush() error {
    results := r.pool.Wait()
    if err := results.FirstError(); err != nil {
        return err
    }

    return nil
}
