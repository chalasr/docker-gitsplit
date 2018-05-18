package gitsplit

import (
    "fmt"
    "strings"
    "github.com/pkg/errors"
    "github.com/libgit2/git2go"
    "github.com/jderusse/gitsplit/utils"
)

type CachePool struct {
    remote *GitRemote
}

type CacheItem struct {
    flagName  string
    sourceId  *git.Oid
    targetId  *git.Oid
}

func NewCachePool(remote *GitRemote) *CachePool {
    return &CachePool{
        remote,
    }
}

func getFlagName(referenceName string, split Split) string {
    return fmt.Sprintf("%s-%s", utils.Hash(referenceName), utils.Hash(strings.Join(split.Prefixes, "-")))
}

func (c *CachePool) SaveItem(item *CacheItem) error {
    if err := c.remote.AddReference("source-"+item.flagName, item.SourceId()); err != nil {
        return errors.Wrapf(err, "Unable to create source reference %s targeting %s", item.flagName, item.SourceId())
    }
    if err := c.remote.AddReference("target-"+item.flagName, item.TargetId()); err != nil {
        return errors.Wrapf(err, "Unable to create target reference %s targeting %s", item.flagName, item.TargetId())
    }

    return nil
}

func (c *CachePool) GetItem(referenceName string, split Split) (*CacheItem, error) {
    flagName := getFlagName(referenceName, split)
    sourceReference, err := c.remote.GetReference("source-"+flagName)
    if err != nil {
        return nil, err
    }

    if sourceReference == nil {
        return &CacheItem{
            flagName: flagName,
        }, nil
    }

    targetReference, err := c.remote.GetReference("target-"+flagName)
    if err != nil {
        return nil, err
    }

    if targetReference == nil {
        return &CacheItem{
            flagName: flagName,
        }, nil
    }

    return &CacheItem{
        flagName: flagName,
        sourceId: sourceReference.Id,
        targetId: targetReference.Id,
    }, nil
}

func (c *CacheItem) IsFresh(reference Reference) bool {
    if c.sourceId == nil || c.targetId == nil {
        return false
    }

    return c.sourceId.Equal(reference.Id)
}

func (c *CacheItem) SourceId() *git.Oid {
    return c.sourceId
}
func (c *CacheItem) TargetId() *git.Oid {
    return c.targetId
}
func (c *CacheItem) Set(sourceId *git.Oid, targetId *git.Oid) {
    c.sourceId = sourceId
    c.targetId = targetId
}