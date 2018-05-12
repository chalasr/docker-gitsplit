package gitsplit

import (
    "os"
    "fmt"
    "regexp"
    "strings"
    "encoding/hex"
    "path/filepath"
    "crypto/sha256"
    "github.com/libgit2/git2go"
    log "github.com/sirupsen/logrus"
    lite "github.com/splitsh/lite/splitter"
    "github.com/jderusse/gitsplit/utils"
)

func hash(input string) string {
    sha_256 := sha256.New()
    sha_256.Write([]byte(input))

    return hex.EncodeToString(sha_256.Sum(nil))
}
func contains(arr []string, str string) bool {
   for _, a := range arr {
      if a == str {
         return true
      }
   }

   return false
}

type Splitter struct {
    config     Config
    repository *git.Repository
}

type Reference struct {
    ShortName string
    Name      string
    Id        *git.Oid
}


func NewSplitter(config Config) (*Splitter, error) {
    initCacheRepository(config.CacheDir)

    splitter := &Splitter {
        config: config,
    }

    var err error
    if splitter.repository, err = git.OpenRepository(config.CacheDir); err != nil {
        return nil, err
    }

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
    references, err := splitter.getReferences("origin")
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
                log.Info("References "+reference.ShortName+" skipped")
                continue
            }

            err := splitter.splitReference(reference)
            if err != nil {
                return fmt.Errorf("Fail to split references: %v", err)
            }
        }
    }

    return nil
}

func (splitter *Splitter) getReferences(remoteName string) ([]Reference, error) {
    result, err := utils.GitExec(splitter.config.CacheDir, "ls-remote", remoteName)
    if err != nil {
        return nil, fmt.Errorf("Fail to fetch references: %v", err)
    }

    references := []Reference{}
    cleanRegexp := regexp.MustCompile("^refs/(tags|heads)/")
    for _, line := range strings.Split(result.Output, "\n") {
        if len(line) == 0 {
            continue
        }
        columns := strings.Split(line, "\t")
        if len(columns) != 2 {
            return nil, fmt.Errorf("Fail to parse reference %s: 2 columns expected", line)
        }
        referenceId := columns[0];
        referenceName := columns[1];
        oid, err := git.NewOid(referenceId)
        if err != nil {
            return nil, fmt.Errorf("Fail to parse reference %s: %v", line, err)
        }
        references = append(references, Reference{
            ShortName: cleanRegexp.ReplaceAllString(referenceName, ""),
            Name: referenceName,
            Id:   oid,
        })
    }

    return references, nil
}

func (splitter *Splitter) splitReference(reference Reference) error {
    for _, split := range splitter.config.Splits {
        flagSource := "refs/split-flag/source-"+hash(reference.Name)+"-"+hash(strings.Join(split.Prefixes, "-"))
        flagTarget := "refs/split-flag/target-"+hash(reference.Name)+"-"+hash(strings.Join(split.Prefixes, "-"))

        splitSourceId, err := splitter.getLocalReference(flagSource)
        if err != nil {
            return err
        }

        if splitSourceId != nil && splitSourceId.Equal(reference.Id) {
            log.Info("Already splitted "+reference.ShortName+" for "+strings.Join(split.Prefixes, ", "))
        } else {
            log.Warn("Splitting "+reference.ShortName+" for "+strings.Join(split.Prefixes, ", "))
            tempBranch := "refs/split/" + reference.Name
            tempReference, err := splitter.repository.References.Create(tempBranch, reference.Id, true, "Temporary reference")
            if err != nil {
                return fmt.Errorf("Unable to create temporary reference %s targeting %s: %v", tempBranch, reference.Id, err)
            }
            defer tempReference.Free()

            splitId, err := splitter.splitLite(tempBranch, split.Prefixes)
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

        for _, target := range split.Targets {
            go func() {
                err := splitter.push(reference, flagTarget, target)
                if err != nil {
                    log.Fatal(fmt.Printf("Unable to push to target %s: %v", target, err))
                }
            }()
        }
    }

    return nil
}

func prefixesToLitePrefixes(prefixes []string) []*lite.Prefix {
    litePrefixes := []*lite.Prefix{}
    for _, prefix := range prefixes {
        parts := strings.Split(prefix, ":")
        from := parts[0]
        to := ""
        if len(parts) > 1 {
            to = parts[1]
        }
        litePrefixes = append(litePrefixes, &lite.Prefix{From: from, To: to})
    }

    return litePrefixes
}


func (splitter *Splitter) push(reference Reference, newReference string, target string) (error) {
    remoteName := hash(target)
    references, err := splitter.getReferences(remoteName)
    if err != nil {
        return fmt.Errorf("Fail to read source references: %v", err)
    }

    newReferenceId, err := splitter.getLocalReference(newReference)
    if err != nil {
        return fmt.Errorf("Unable to locate split result: %v", err)
    }
    if newReferenceId == nil {
        return fmt.Errorf("Unable to locate split result")
    }
    for _, remoteReference := range references {
        if remoteReference.Name == reference.Name {
            if remoteReference.Id.Equal(newReferenceId) {
                log.Info("Already pushed "+reference.ShortName+" into "+target)
                return nil
            }
            log.Warn("Out of date "+reference.ShortName+" into "+target)
            break
        }
    }

    log.Warn("Pushing "+reference.ShortName+" into "+target)


    if _, err := utils.GitExec(splitter.config.CacheDir, "push", "-f", remoteName, newReferenceId.String()+":"+reference.Name); err != nil {
        return fmt.Errorf("Fail to push: %v", err)
    }

    return nil
}

func (splitter *Splitter) splitLite(origin string, prefixes []string) (*git.Oid, error) {
    config := &lite.Config{
        Path:       splitter.config.CacheDir,
        Origin:     origin,
        Prefixes:   prefixesToLitePrefixes(prefixes),
        Target:     "",
        Commit:     "",
        Debug:      false,
        Scratch:    false,
        GitVersion: "latest",
    }

    result := &lite.Result{}
    err := lite.Split(config, result)
    if err != nil {
        return nil, err
    }

    return result.Head(), nil
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
    remotes, err := splitter.repository.Remotes.List()
    if err != nil {
        return fmt.Errorf("Unable to list remotes: %v", err)
    }

    // Configure cache
    if !contains(remotes, "origin") {
        splitter.repository.Remotes.Create("origin", "file://" + splitter.config.ProjectDir)
    }

    log.Info("Updating cache from source")
    splitter.repository.Remotes.SetUrl("origin", "file://" + splitter.config.ProjectDir)

    // Updating cache
    if _, err := utils.GitExec(splitter.config.CacheDir, "fetch", "-p", "origin"); err != nil {
        return fmt.Errorf("Fail to update cache: %v", err)
    }
    if _, err := utils.GitExec(splitter.config.CacheDir, "fetch", "--tags", "origin"); err != nil {
        return fmt.Errorf("Fail to update cache: %v", err)
    }


    knownRemotes := []string{"origin"}

    // Configure cache"s remotes
    for _, split := range splitter.config.Splits {
        for _, target := range split.Targets {
            remoteName := hash(target)
            if !contains(knownRemotes, remoteName) {
                knownRemotes = append(knownRemotes, remoteName)
            }

            targetUrl := os.ExpandEnv(target)
            if !contains(remotes, remoteName) {
                splitter.repository.Remotes.Create(remoteName, targetUrl)
            }

            log.Info("Updating cache from target ", target)
            splitter.repository.Remotes.SetUrl(remoteName, targetUrl)

            if _, err := utils.GitExec(splitter.config.CacheDir, "fetch", "-p", remoteName); err != nil {
                return fmt.Errorf("Fail to update cache: %v", err)
            }
            if _, err := utils.GitExec(splitter.config.CacheDir, "fetch", "--tags", remoteName); err != nil {
                return fmt.Errorf("Fail to update cache: %v", err)
            }
        }
    }

    // Cleanup old cache"s remotes
    for _, remoteName := range remotes {
        if !contains(knownRemotes, remoteName) {
            log.Info("Removing remote ", remoteName)
            splitter.repository.Remotes.Delete(remoteName)
        }
    }

    return nil
}
