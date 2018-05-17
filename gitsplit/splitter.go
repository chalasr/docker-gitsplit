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
    flagSuffix := utils.Hash(reference.Name)+"-"+utils.Hash(strings.Join(split.Prefixes, "-"))
    sourceReferenceName := "source-"+flagSuffix
    targetReferenceName := "target-"+flagSuffix
    flagTemp := "refs/split-temp/"+flagSuffix

    remote, err := s.workingSpace.Remotes().Get("cache")
    if err != nil {
        return err
    }
    sourceReference, err := remote.GetReference(sourceReferenceName)
    if err != nil {
        return err
    }

    if sourceReference != nil && sourceReference.Id.Equal(reference.Id) {
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

        if err := remote.AddReference(sourceReferenceName, reference.Id); err != nil {
            return errors.Wrapf(err, "Unable to create target reference %s targeting %s", targetReferenceName, splitId)
        }
        if err := remote.AddReference(targetReferenceName, splitId); err != nil {
            return errors.Wrapf(err, "Unable to create target reference %s targeting %s", targetReferenceName, splitId)
        }
    }

    targetReference, err := remote.GetReference(targetReferenceName)
    if err != nil {
        return err
    }
    if targetReference == nil {
        return errors.Wrap(err, "Unable to locate split result")
    }

    for _, target := range split.Targets {
        remote, err := s.workingSpace.Remotes().Get(target)
        if err != nil {
            return err
        }
        if err := remote.Push(reference, targetReference.Id); err != nil {
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
