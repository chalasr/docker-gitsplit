package gitsplit

import (
    "regexp"
    "strings"
    "github.com/libgit2/git2go"
    log "github.com/sirupsen/logrus"
    "github.com/pkg/errors"
    "github.com/jderusse/gitsplit/utils"
)

type Reference struct {
    Alias     string
    ShortName string
    Name      string
    Id        *git.Oid
}

type Splitter struct {
    config            Config
    referenceSplitter *ReferenceSplitterLite
    workingSpace      *WorkingSpace
}

func NewSplitter(config Config, workingSpace *WorkingSpace) *Splitter {
    return &Splitter {
        config:             config,
        workingSpace:       workingSpace,
        referenceSplitter:  NewReferenceSplitterLite(workingSpace.Repository()),
    }
}

func (s *Splitter) Split(whitelistReferences []string) error {
    remote, err := s.workingSpace.Remotes().Get("origin")
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
            if !referenceRegexp.MatchString(reference.Alias) {
                continue
            }
            if len(whitelistReferences) > 0 && !utils.InArray(whitelistReferences, reference.Alias) {
                continue
            }

            for _, split := range s.config.Splits {
                if err := s.splitReference(reference, split); err != nil {
                    return errors.Wrap(err, "Fail to split references")
                }
            }
        }
    }

    if err := s.workingSpace.Remotes().Flush(); err != nil {
        return errors.Wrap(err, "Fail to flush references")
    }
    return nil
}

func (s *Splitter) splitReference(reference Reference, split Split) error {
    flagTemp := "refs/split-temp/"+utils.Hash(reference.Name)+"-"+utils.Hash(strings.Join(split.Prefixes, "-"))

    remote, err := s.workingSpace.Remotes().Get("cache")
    if err != nil {
        return err
    }
    cachePool := NewCachePool(remote)


    previousReference, err := cachePool.GetItem(reference.Name, split)
    if err != nil {
        return errors.Wrap(err, "Fail to fetch previousReference metadata")
    }

    if previousReference.IsFresh(reference) {
        log.Info("Already splitted "+reference.Alias+" for "+strings.Join(split.Prefixes, ", "))
    } else {
        log.Warn("Splitting "+reference.Alias+" for "+strings.Join(split.Prefixes, ", "))
        tempReference, err := s.workingSpace.Repository().References.Create(flagTemp, reference.Id, true, "Temporary reference")
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

        previousReference.Set(reference.Id, splitId)
        if err := cachePool.SaveItem(previousReference); err != nil {
            return errors.Wrapf(err, "Fail to cache reference %s targeting %s", flagTemp)
        }
    }

    for _, target := range split.Targets {
        remote, err := s.workingSpace.Remotes().Get(target)
        if err != nil {
            return err
        }
        if err := remote.Push(reference, previousReference.TargetId()); err != nil {
            return err
        }
    }

    return nil
}

func (s *Splitter) getLocalReference(referenceName string) (*git.Oid, error) {
    reference, err := s.workingSpace.Repository().References.Dwim(referenceName)
    if err != nil {
        return nil, nil
    }

    return reference.Target(), nil
}
