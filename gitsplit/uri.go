package gitsplit

import (
    "os"
    "strings"
    "github.com/jderusse/gitsplit/utils"
)


type GitUri struct {
    scheme string
    uri    string
}

func (u *GitUri) IsLocal() bool {
    return u.scheme == "file"
}

func (u *GitUri) Uri() string {
    if u.scheme == "" {
        return u.SchemelessUri()
    }

    return u.scheme + "://" + u.SchemelessUri()
}

func (u *GitUri) SchemelessUri() string {
    if u.IsLocal() {
        return utils.ResolvePath(u.uri)
    }

    return os.ExpandEnv(u.uri)
}

func ParseUri(uri string) *GitUri {
    parts := strings.SplitN(uri, "://", 2)
    if len(parts) == 2 {
        return &GitUri{
            scheme: parts[0],
            uri: parts[1],
        }
    }

    parts = strings.SplitN(uri, "/", 2)
    if strings.Index(parts[0], ":") > 0 {
        return &GitUri{
            scheme: "",
            uri: uri,
        }
    }

    return &GitUri{
        scheme: "file",
        uri: uri,
    }
}
