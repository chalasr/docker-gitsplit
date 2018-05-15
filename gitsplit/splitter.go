package gitsplit

import (
    "os"
    "fmt"
    "regexp"
    "strings"
    "path/filepath"
    "github.com/libgit2/git2go"
    log "github.com/sirupsen/logrus"
    "github.com/jderusse/gitsplit/utils"
)

func contains(arr []string, str string) bool {
   for _, a := range arr {
      if a == str {
         return true
      }
   }

   return false
}

type Reference struct {
    ShortName string
    Name      string
    Id        *git.Oid
}

type Splitter struct {
    config            Config
    repository        *git.Repository
    remotes           *GitRemoteCollection
    referenceSplitter ReferenceSplitterInterface
}


func NewSplitter(config Config, referenceSplitterFactory ReferenceSplitterFactoryInterface) (*Splitter, error) {
    initCacheRepository(config.CacheDir)

    splitter := &Splitter {
        config:  config,
    }

    var err error
    if splitter.repository, err = git.OpenRepository(config.CacheDir); err != nil {
        return nil, err
    }
    splitter.remotes = NewGitRemoteCollection(splitter.repository)
    splitter.referenceSplitter = referenceSplitterFactory.New(splitter.repository)

    return splitter, nil
}

func (splitter *Splitter) Close() error {
    splitter.repository.Free()
    return nil
}


func (splitter *Splitter) Split(whitelistReferences []string) error {
    if err := splitter.initWorkspace(); err != nil {
        return err
    }
    if err := splitter.splitReferences(whitelistReferences); err != nil {
        return err
    }

    return nil
}

func (splitter *Splitter) splitReferences(whitelistReferences []string) error {
    remote, err := splitter.remotes.Get("origin")
    if err != nil {
        return err
    }

    references, err := remote.GetReferences()
    if err != nil {
        return fmt.Errorf("Fail to read source references: %v", err)
    }
    for _, reference := range references {
        if len(reference.Name) == 0 {
            continue
        }
        for _, referencePattern := range splitter.config.Origins {
            referenceRegexp := regexp.MustCompile(referencePattern)
            if !referenceRegexp.MatchString(reference.ShortName) {
                continue
            }
            if len(whitelistReferences) > 0 && !contains(whitelistReferences, reference.ShortName) {
                continue
            }

            for _, split := range splitter.config.Splits {
                if err := splitter.splitReference(reference, split); err != nil {
                    return fmt.Errorf("Fail to split references: %v", err)
                }
            }
        }
    }

    if err := splitter.remotes.Flush(); err != nil {
        return fmt.Errorf("Fail to flush references: %v", err)
    }
    return nil
}

func (splitter *Splitter) splitReference(reference Reference, split Split) error {
    flagSource := "refs/split-flag/source-"+utils.Hash(reference.Name)+"-"+utils.Hash(strings.Join(split.Prefixes, "-"))
    flagTarget := "refs/split-flag/target-"+utils.Hash(reference.Name)+"-"+utils.Hash(strings.Join(split.Prefixes, "-"))
    flagTemp := "refs/split-flag/temp-"+utils.Hash(reference.Name)+"-"+utils.Hash(strings.Join(split.Prefixes, "-"))

    splitSourceId, err := splitter.getLocalReference(flagSource)
    if err != nil {
        return err
    }

    if splitSourceId != nil && splitSourceId.Equal(reference.Id) {
        log.Info("Already splitted "+reference.ShortName+" for "+strings.Join(split.Prefixes, ", "))
    } else {
        log.Warn("Splitting "+reference.ShortName+" for "+strings.Join(split.Prefixes, ", "))
        tempReference, err := splitter.repository.References.Create(flagTemp, reference.Id, true, "Temporary reference")
        if err != nil {
            return fmt.Errorf("Unable to create temporary reference %s targeting %s: %v", flagTemp, reference.Id, err)
        }
        defer tempReference.Free()

        splitId, err := splitter.referenceSplitter.Split(flagTemp, split.Prefixes)
        if err != nil {
            return fmt.Errorf("Unable split reference: %v", err)
        }

        err = tempReference.Delete()
        if err != nil {
            return fmt.Errorf("Unable to cleanup temporary reference %s targeting %s: %v", flagTarget, splitId, err)
        }

        targetRef, err := splitter.repository.References.Create(flagTarget, splitId, true, "Flag target reference")
        if err != nil {
            return fmt.Errorf("Unable to create target reference %s targeting %s: %v", flagTarget, splitId, err)
        }
        targetRef.Free()

        sourceRef, err := splitter.repository.References.Create(flagSource, reference.Id, true, "Flag source reference")
        if err != nil {
            return fmt.Errorf("Unable to create source reference %s targeting %s: %v", flagSource, reference.Id, err)
        }
        sourceRef.Free()
    }

    splitId, err := splitter.getLocalReference(flagTarget)
    if err != nil {
        return fmt.Errorf("Unable to locate split result: %v", err)
    }
    if splitId == nil {
        return fmt.Errorf("Unable to locate split result")
    }

    for _, target := range split.Targets {
        remote, err := splitter.remotes.Get(target)
        if err != nil {
            return err
        }
        remote.Push(reference, splitId, target)
    }

    return nil
}

func (splitter *Splitter) getLocalReference(referenceName string) (*git.Oid, error) {
    reference, err := splitter.repository.References.Dwim(referenceName)
    if err != nil {
        return nil, nil
    }

    return reference.Target(), nil
}

func initCacheRepository(cacheDir string) {
    if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
        log.Info("Initializing cache repository")
        dir, _ := filepath.Split(cacheDir)
        git.InitRepository(dir, false)
    }
}

func (splitter *Splitter) initWorkspace() error {
    remote := splitter.remotes.Add("origin", "file://" + splitter.config.ProjectDir)
    remote.Fetch()

    for _, split := range splitter.config.Splits {
        for _, target := range split.Targets {
            remote := splitter.remotes.Add(target, target)
            remote.Fetch()
        }
    }
    go splitter.remotes.Clean()

    if err := splitter.remotes.Flush(); err != nil {
        return err
    }

    return nil
}
