package gitsplit

import (
    "os"
    "regexp"
    "strings"
    "path/filepath"
    "github.com/libgit2/git2go"
    log "github.com/sirupsen/logrus"
    "github.com/pkg/errors"
    "github.com/jderusse/gitsplit/utils"
)

type Reference struct {
    ShortName string
    Name      string
    Id        *git.Oid
}

type Splitter struct {
    config            Config
    repository        *git.Repository
    remotes           *GitRemoteCollection
    referenceSplitter *ReferenceSplitterLite
}


func NewSplitter(config Config, repository *git.Repository) *Splitter {
    return &Splitter {
        config:             config,
        repository:         repository,
        remotes:            NewGitRemoteCollection(repository),
        referenceSplitter:  NewReferenceSplitterLite(repository),
    }
}

func (s *Splitter) Close() error {
    s.repository.Free()
    return nil
}


func (s *Splitter) Split(whitelistReferences []string) error {
    if err := s.initWorkspace(); err != nil {
        return err
    }
    if err := s.splitReferences(whitelistReferences); err != nil {
        return err
    }

    return nil
}

func (s *Splitter) splitReferences(whitelistReferences []string) error {
    remote, err := s.remotes.Get("origin")
    if err != nil {
        return err
    }

    references, err := remote.GetReferences()
    if err != nil {
        return errors.Wrap(err, "Fail to read source references")
    }
    for _, reference := range references {
        if len(reference.Name) == 0 {
            continue
        }
        for _, referencePattern := range s.config.Origins {
            referenceRegexp := regexp.MustCompile(referencePattern)
            if !referenceRegexp.MatchString(reference.ShortName) {
                continue
            }
            if len(whitelistReferences) > 0 && !utils.InArray(whitelistReferences, reference.ShortName) {
                continue
            }

            for _, split := range s.config.Splits {
                if err := s.splitReference(reference, split); err != nil {
                    return errors.Wrap(err, "Fail to split references")
                }
            }
        }
    }

    if err := s.remotes.Flush(); err != nil {
        return errors.Wrap(err, "Fail to flush references")
    }
    return nil
}

func (s *Splitter) splitReference(reference Reference, split Split) error {
    flagSuffix := utils.Hash(reference.Name)+"-"+utils.Hash(strings.Join(split.Prefixes, "-"))
    flagSource := "refs/split-flag/source-"+flagSuffix
    flagTarget := "refs/split-flag/target-"+flagSuffix
    flagTemp := "refs/split-flag/temp-"+flagSuffix

    splitSourceId, err := s.getLocalReference(flagSource)
    if err != nil {
        return err
    }

    if splitSourceId != nil && splitSourceId.Equal(reference.Id) {
        log.Info("Already splitted "+reference.ShortName+" for "+strings.Join(split.Prefixes, ", "))
    } else {
        log.Warn("Splitting "+reference.ShortName+" for "+strings.Join(split.Prefixes, ", "))
        tempReference, err := s.repository.References.Create(flagTemp, reference.Id, true, "Temporary reference")
        if err != nil {
            return errors.Wrapf(err, "Unable to create temporary reference %s targeting %s", flagTemp, reference.Id)
        }
        defer tempReference.Free()

        splitId, err := s.referenceSplitter.Split(flagTemp, split.Prefixes)
        if err != nil {
            return errors.Wrap(err, "Unable split reference")
        }

        err = tempReference.Delete()
        if err != nil {
            return errors.Wrapf(err, "Unable to delete temporary reference %s targeting %s", flagTemp, splitId)
        }

        targetRef, err := s.repository.References.Create(flagTarget, splitId, true, "Flag target reference")
        if err != nil {
            return errors.Wrapf(err, "Unable to create target reference %s targeting %s", flagTarget, splitId)
        }
        targetRef.Free()

        sourceRef, err := s.repository.References.Create(flagSource, reference.Id, true, "Flag source reference")
        if err != nil {
            return errors.Wrapf(err, "Unable to create source reference %s targeting %s", flagSource, reference.Id)
        }
        sourceRef.Free()
    }

    splitId, err := s.getLocalReference(flagTarget)
    if err != nil {
        return errors.Wrap(err, "Unable to locate split result")
    }
    if splitId == nil {
        return errors.Wrap(err, "Unable to locate split result")
    }

    for _, target := range split.Targets {
        remote, err := s.remotes.Get(target)
        if err != nil {
            return err
        }
        remote.Push(reference, splitId, target)
    }

    return nil
}

func (s *Splitter) getLocalReference(referenceName string) (*git.Oid, error) {
    reference, err := s.repository.References.Dwim(referenceName)
    if err != nil {
        return nil, nil
    }

    return reference.Target(), nil
}

func (s *Splitter) initWorkspace() error {
    remote := s.remotes.Add("origin", "file://" + s.config.ProjectDir)
    remote.Fetch()

    for _, split := range s.config.Splits {
        for _, target := range split.Targets {
            remote := s.remotes.Add(target, target)
            remote.Fetch()
        }
    }
    go s.remotes.Clean()

    if err := s.remotes.Flush(); err != nil {
        return err
    }

    return nil
}

func GetCacheRepository(cacheDir string) (*git.Repository, error) {
    if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
        log.Info("Initializing cache repository")
        dir, _ := filepath.Split(cacheDir)
        git.InitRepository(dir, false)
    }

    return git.OpenRepository(cacheDir)
}