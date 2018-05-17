package gitsplit

import (
    "os"
    "fmt"
    "strings"
    "io/ioutil"
    "gopkg.in/yaml.v2"
    "path/filepath"
    "github.com/pkg/errors"
)

type StringCollection []string

type Split struct {
    Prefixes StringCollection  `yaml:"prefix"`
    Targets  StringCollection  `yaml:"target"`
}

type Config struct {
    CacheDir   string    `yaml:"cache_dir"`
    ProjectDir string    `yaml:"project_dir"`
    Splits     []Split   `yaml:"splits"`
    Origins    []string  `yaml:"origins"`
}

func (s *StringCollection) UnmarshalYAML(unmarshal func(interface{}) error) error {
    var raw interface{}
    if err := unmarshal(& raw); err != nil {
        return err
    }


    switch raw.(type){
        case string:
            *s = []string{raw.(string)}
        case []string:
            *s = raw.([]string)
        default:
            return fmt.Errorf("expects a string or n array of strings")
    }

    return nil
}

func resolvePath(path string) string {
    path = os.ExpandEnv(path)
    if path =="~" || strings.HasPrefix(path, "~/") {
        path = strings.Replace(path, "~", os.Getenv("HOME"), 1)
    }

    if filepath.IsAbs(path) {
        return path
    }

    pwd, err := os.Getwd()
    if err != nil {
        return path
    }

    return filepath.Join(pwd, path)
}

func NewConfigFromFile(filePath string) (*Config, error) {
    config := &Config{}

    yamlFile, err := ioutil.ReadFile(resolvePath(filePath))
    if err != nil {
        return nil, errors.Wrap(err, "Fail to read config file")
    }

    err = yaml.Unmarshal(yamlFile, &config)
    if err != nil {
        return nil, errors.Wrap(err, "Fail to load config file")
    }

    if config.ProjectDir == "" {
        config.ProjectDir = resolvePath(".")
    }

    config.CacheDir = resolvePath(config.CacheDir)
    if len(config.Origins) == 0 {
        config.Origins =  []string{".*"}
    }

    return config, nil
}
